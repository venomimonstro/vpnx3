package ru.vpnx3.app.data

import android.content.Context
import android.os.Build
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
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
    val sessionId: String,
    val configVersion: Long
)

class VpnRepository(private val context: Context) {
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
        state.resetRequestSequence()
        return registration
    }

    fun obtainLease(tunnelPublicKey: String): AccessLease {
        val registration = ensureRegistered()
        val next = state.reserveNextRequestSequence()
        return api.lease(registration.deviceId, next, tunnelPublicKey)
    }

    fun latestConfig(): VerifiedConfig = configRepository.refreshOrFallback()

    fun plans(): List<ClientPlan> = api.plans()

    fun accountStatus(): ClientAccountStatus {
        val registration=ensureRegistered()
        val sequence=state.reserveNextRequestSequence()
        return api.accountStatus(registration.deviceId,sequence)
    }

    fun createPairingCode(): PairingCode {
        val registration=ensureRegistered()
        val sequence=state.reserveNextRequestSequence()
        return api.createPairingCode(registration.deviceId,sequence)
    }

    fun claimPairingCode(code:String): ClientAccountStatus {
        val registration=ensureRegistered()
        val sequence=state.reserveNextRequestSequence()
        val status=api.claimPairingCode(registration.deviceId,sequence,code)
        state.userId=status.userId
        state.trialExpiresAt=null
        return status
    }

    fun createPayment(planId:String): PaymentStart {
        val registration=ensureRegistered()
        val sequence=state.reserveNextRequestSequence()
        return api.createPayment(registration.deviceId,sequence,planId)
    }

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
                return PreparedConnection(config, route, session.sessionId, signedConfig.version)
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
        val started=System.currentTimeMillis()
        try {
            wireGuard.connect(prepared.config)
            state.saveActiveSession(prepared)
            active = prepared
            reportTelemetry("connect_success",prepared.configVersion,prepared.workerRoute.nodeId,System.currentTimeMillis()-started)
        } catch (t:Throwable) {
            reportTelemetry("connect_failed",prepared.configVersion,prepared.workerRoute.nodeId,System.currentTimeMillis()-started)
            throw t
        }
    }

    fun disconnect() {
        val current = active
        val persisted = state.activeSession()
        runCatching { wireGuard.disconnect() }

        if (current != null) {
            release(current)
        } else if (persisted != null) {
            val endpoint=NetworkEndpoint(
                scheme=persisted.scheme,
                host=persisted.host,
                port=persisted.port,
                path=persisted.path,
                priority=persisted.priority
            )
            runCatching { workerApi.closeSession(endpoint,persisted.sessionId) }
        }

        state.clearActiveSession()
        active = null
        reportTelemetry("disconnect")
    }

    fun reportTelemetry(eventType:String,configVersion:Long=0,workerNodeId:String?=null,durationMS:Long=0) {
        runCatching {
            val registration=ensureRegistered()
            val sequence=state.reserveNextRequestSequence()
            api.telemetry(
                registration.deviceId,sequence,eventType,configVersion,workerNodeId,
                networkType(),durationMS
            )
        }
    }

    private fun networkType():String {
        val cm=context.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
        val caps=cm.getNetworkCapabilities(cm.activeNetwork) ?: return "unknown"
        return when {
            caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
            caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
            caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
            caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN) -> "vpn"
            else -> "unknown"
        }
    }

    fun isConnected(): Boolean = wireGuard.isConnected()

    fun hasPersistedSession(): Boolean = state.activeSession()!=null

    fun recoverConnectionState(): Boolean {
        val connected=wireGuard.isConnected()
        if(!connected && state.activeSession()!=null) {
            // Process/backend died while worker session metadata survived.
            // Close the stale server-side peer instead of pretending the tunnel is alive.
            val persisted=state.activeSession()!!
            val endpoint=NetworkEndpoint(
                scheme=persisted.scheme,
                host=persisted.host,
                port=persisted.port,
                path=persisted.path,
                priority=persisted.priority
            )
            runCatching { workerApi.closeSession(endpoint,persisted.sessionId) }
            state.clearActiveSession()
            reportTelemetry("stale_session_cleaned")
        } else if(connected) {
            reportTelemetry("recovered")
        }
        return connected
    }
}
