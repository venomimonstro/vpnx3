package ru.vpnx3.app.data

import android.content.Context
import android.os.Build
import com.wireguard.config.Config
import org.json.JSONArray
import ru.vpnx3.app.BuildConfig
import ru.vpnx3.app.security.DeviceIdentity
import ru.vpnx3.app.security.TunnelKeyStore
import ru.vpnx3.app.security.VerifiedConfig
import ru.vpnx3.app.vpn.WireGuardController
import java.io.ByteArrayInputStream

data class PreparedConnection(
    val config: Config,
    val workerRoute: WorkerRoute,
    val sessionId: String
)

class VpnRepository(context: Context) {
    private val state = LocalState(context)
    private val identity = DeviceIdentity()
    private val api = ControlApi(BuildConfig.CONTROL_URL, identity)
    private val workerApi = WorkerApi()
    private val tunnelKeys = TunnelKeyStore(context)
    private val wireGuard = WireGuardController(context)
    private val configRepository: ConfigRepository

    @Volatile
    private var active: PreparedConnection? = null

    init {
        require(BuildConfig.CONFIG_PUBLIC_KEY.isNotBlank()) {
            "VPNX3_CONFIG_PUBLIC_KEY must be pinned at build time"
        }
        configRepository = ConfigRepository(api, state, BuildConfig.CONFIG_PUBLIC_KEY)
    }

    fun ensureRegistered(): Registration {
        val existingDevice = state.deviceId
        val existingUser = state.userId
        if (existingDevice != null && existingUser != null) {
            return Registration(existingUser, existingDevice, state.trialExpiresAt.orEmpty())
        }

        val name = "${Build.MANUFACTURER} ${Build.MODEL}".trim()
        val registration = api.register(name.ifEmpty { "Android" })
        state.userId = registration.userId
        state.deviceId = registration.deviceId
        state.trialExpiresAt = registration.trialExpiresAt
        state.requestSequence = 0L
        return registration
    }

    fun obtainLease(tunnelPublicKey: String): AccessLease {
        val registration = ensureRegistered()
        val next = state.requestSequence + 1
        val lease = api.lease(registration.deviceId, next, tunnelPublicKey)
        state.requestSequence = next
        return lease
    }

    fun latestConfig(): VerifiedConfig = configRepository.refreshOrFallback()

    fun prepareConnection(): PreparedConnection {
        val signedConfig = latestConfig()
        val routes = RoutingSelector.candidates(signedConfig)
        require(routes.isNotEmpty()) { "No active WireGuard worker is available" }

        val keyPair = tunnelKeys.getOrCreate()
        val tunnelPublicKey = keyPair.publicKey.toBase64()
        val lease = obtainLease(tunnelPublicKey)
        val network = signedConfig.payload.optJSONObject("network")
        val mtu = network?.optInt("mtu", 1280)?.coerceIn(576, 1500) ?: 1280
        val keepalive = network?.optInt("persistent_keepalive_seconds", 25)
            ?.coerceIn(0, 120) ?: 25
        val dns = network?.optJSONArray("dns_servers") ?: JSONArray()

        var lastError: Throwable? = null
        for (route in routes) {
            try {
                val session = workerApi.createSession(
                    endpoint = route.sessionApi,
                    leaseEnvelope = lease.rawEnvelope,
                    clientPublicKey = tunnelPublicKey
                )

                if (session.endpoint != route.wireGuard.hostPort()) {
                    runCatching { workerApi.closeSession(route.sessionApi, session.sessionId) }
                    error("Worker returned endpoint that differs from signed configuration")
                }

                val address = if (session.assignedIp.contains('/')) {
                    session.assignedIp
                } else {
                    "${session.assignedIp}/32"
                }

                val dnsLine = if (dns.length() > 0) {
                    val values = (0 until dns.length()).joinToString(", ") { dns.getString(it) }
                    "DNS = $values\n"
                } else {
                    ""
                }

                val wgQuick = """
                    [Interface]
                    PrivateKey = ${keyPair.privateKey.toBase64()}
                    Address = $address
                    MTU = $mtu
                    $dnsLine
                    [Peer]
                    PublicKey = ${session.serverPublicKey}
                    AllowedIPs = 0.0.0.0/0, ::/0
                    Endpoint = ${session.endpoint}
                    PersistentKeepalive = $keepalive
                """.trimIndent()

                val config = Config.parse(
                    ByteArrayInputStream(wgQuick.toByteArray(Charsets.UTF_8))
                )
                return PreparedConnection(config, route, session.sessionId)
            } catch (error: Throwable) {
                lastError = error
            }
        }
        throw IllegalStateException("All configured workers failed", lastError)
    }

    fun release(prepared: PreparedConnection) {
        runCatching {
            workerApi.closeSession(prepared.workerRoute.sessionApi, prepared.sessionId)
        }
    }

    fun connect(prepared: PreparedConnection) {
        wireGuard.connect(prepared.config)
        active = prepared
    }

    fun disconnect() {
        val current = active
        runCatching { wireGuard.disconnect() }
        if (current != null) release(current)
        active = null
    }

    fun isConnected(): Boolean = wireGuard.isConnected()
}
