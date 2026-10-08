package ru.vpnx3.app.data

import ru.vpnx3.app.security.TrustBundleVerifier
import ru.vpnx3.app.security.VerifiedTrustBundle
import java.time.Instant

class TrustRepository(
    private val api:ControlApi,
    private val state:LocalState,
    rootPublicKey:String
) {
    private val verifier=TrustBundleVerifier(rootPublicKey)

    fun refreshOrFallback(now:Instant=Instant.now()):VerifiedTrustBundle {
        val minimum=state.highestTrustBundleVersion
        val remote=runCatching { api.trustBundle() }
            .mapCatching { accept(it,minimum,now) }
        if(remote.isSuccess) return remote.getOrThrow()

        val cached=state.trustBundleEnvelope
            ?: throw IllegalStateException("No valid trust bundle available",remote.exceptionOrNull())
        return verifier.verify(cached,state.highestTrustBundleVersion,now)
    }

    private fun accept(raw:String,minimum:Long,now:Instant):VerifiedTrustBundle {
        val verified=verifier.verify(raw,minimum,now)
        state.trustBundleEnvelope=raw
        state.highestTrustBundleVersion=maxOf(minimum,verified.version)
        return verified
    }
}
