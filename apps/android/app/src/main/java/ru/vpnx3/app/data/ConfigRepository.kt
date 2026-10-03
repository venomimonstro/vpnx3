package ru.vpnx3.app.data

import ru.vpnx3.app.security.ConfigVerifier
import ru.vpnx3.app.security.VerifiedConfig
import java.time.Instant

class ConfigRepository(
    private val api: ControlApi,
    private val state: LocalState,
    publicKey: String
) {
    private val verifier = ConfigVerifier(publicKey)

    fun refreshOrFallback(now: Instant = Instant.now()): VerifiedConfig {
        val minimum = state.highestConfigVersion

        val remote = runCatching { api.latestConfig() }
            .mapCatching { raw ->
                val verified = verifier.verify(raw, minimum, now)
                state.configEnvelope = raw
                state.highestConfigVersion = maxOf(minimum, verified.version)
                verified
            }

        if (remote.isSuccess) {
            return remote.getOrThrow()
        }

        val cached = state.configEnvelope
            ?: throw IllegalStateException("No valid configuration available", remote.exceptionOrNull())

        return verifier.verify(
            rawEnvelope = cached,
            minimumVersion = state.highestConfigVersion,
            now = now
        )
    }
}
