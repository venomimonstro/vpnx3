package ru.vpnx3.app.data

import android.content.Context
import android.os.Build
import com.wireguard.config.Config
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
        configRepository = ConfigRepository(
            api = api,
            state = state,
            publicKey = BuildConfig.CONFIG_PUBLIC_KEY
        )
    }

    fun ensureRegistered(): Registration {
        val existingDevice = state.deviceId
        val existingUser = state.userId
        if (existingDevice != null && existingUser != null) {
            return Registration(
                userId = existingUser,
                deviceId = existingDevice,
                trialExpiresAt = state.trialExpiresAt.orEmpty()
            )
        }

        val name = "${Build.MANUFACTURER} ${Build.MODEL}".trim()
        val registration = api.register(name.ifEmpty { "Android" })
        state.userId = registration.userId
        state.deviceId = registration.deviceId
        state.trialExpiresAt = registration.trialExpiresAt
        state.requestSequence = 0L
        return registration
    }

    fun obtainLease(): AccessLease {
        val registration = ensureRegistered()
        val next = state.requestSequence + 1
        val lease = api.lease(registration.deviceId, next)
        state.requestSequence = next
        return lease
    }

    fun latestConfig(): VerifiedConfig = configRepository.refreshOrFallback()

    fun prepareConnection(): PreparedConnection {
        val signedConfig = latestConfig()
        val route = RoutingSelector.select(signedConfig)
        val lease = obtainLease()
        val keyPair = tunnelKeys.getOrCreate()

        val session = workerApi.createSession(
            endpoint = route.sessionApi,
            leaseEnvelope = lease.rawEnvelope,
            clientPublicKey = keyPair.publicKey.toBase64()
        )

        require(session.endpoint == route.wireGuard.hostPort()) {
            runCatching { workerApi.closeSession(route.sessionApi, session.sessionId) }
            "Worker returned WireGuard endpoint that differs from signed configuration"
        }

        val address = if (session.assignedIp.contains('/')) {
            session.assignedIp
        } else {
            "${session.assignedIp}/32"
        }

        val wgQuick = """
            [Interface]
            PrivateKey = ${keyPair.privateKey.toBase64()}
            Address = $address

            [Peer]
            PublicKey = ${session.serverPublicKey}
            AllowedIPs = 0.0.0.0/0, ::/0
            Endpoint = ${session.endpoint}
            PersistentKeepalive = 25
        """.trimIndent()

        val config = Config.parse(
            ByteArrayInputStream(wgQuick.toByteArray(Charsets.UTF_8))
        )
        return PreparedConnection(
            config = config,
            workerRoute = route,
            sessionId = session.sessionId
        )
    }

    fun connect(prepared: PreparedConnection) {
        wireGuard.connect(prepared.config)
        active = prepared
    }

    fun disconnect() {
        val current = active
        runCatching { wireGuard.disconnect() }
        if (current != null) {
            runCatching {
                workerApi.closeSession(
                    current.workerRoute.sessionApi,
                    current.sessionId
                )
            }
        }
        active = null
    }

    fun isConnected(): Boolean = wireGuard.isConnected()
}
