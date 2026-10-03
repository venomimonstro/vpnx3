package ru.vpnx3.app.data

import android.content.Context
import android.os.Build
import ru.vpnx3.app.BuildConfig
import ru.vpnx3.app.security.DeviceIdentity
import ru.vpnx3.app.security.VerifiedConfig

class VpnRepository(context: Context) {
    private val state = LocalState(context)
    private val identity = DeviceIdentity()
    private val api = ControlApi(BuildConfig.CONTROL_URL, identity)
    private val configRepository: ConfigRepository

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
}
