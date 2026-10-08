package ru.vpnx3.app.data

import org.json.JSONObject
import ru.vpnx3.app.security.ConfigVerifier
import ru.vpnx3.app.security.VerifiedConfig
import java.time.Instant

class ConfigRepository(
    private val api: ControlApi,
    private val state: LocalState,
    publicKey: String,
    bootstrapUrls: String = "",
    private val trustRepository:TrustRepository? = null
) {
    private val legacyVerifier = publicKey.takeIf{it.isNotBlank()}?.let(::ConfigVerifier)
    private val bootstrap = bootstrapUrls
        .split(',', ';')
        .map { it.trim() }
        .filter { it.startsWith("https://") }
        .distinct()
        .take(16)

    fun refreshOrFallback(now: Instant = Instant.now()): VerifiedConfig {
        val minimum = state.highestConfigVersion
        var lastError: Throwable? = null

        // Primary Control Plane remains first. Mirrors are data sources only:
        // every response must pass the same pinned Ed25519/rollback/expiry verification.
        val primary = runCatching { api.latestConfig() }
            .mapCatching { accept(it, minimum, now) }
        if (primary.isSuccess) return primary.getOrThrow()
        lastError = primary.exceptionOrNull()

        val mirrors = (bootstrap + state.configMirrorUrls).distinct().take(32)
        for (url in mirrors) {
            val result = runCatching { api.latestConfigFrom(url) }
                .mapCatching { accept(it, minimum, now) }
            if (result.isSuccess) return result.getOrThrow()
            lastError = result.exceptionOrNull() ?: lastError
        }

        val cached = state.configEnvelope
            ?: throw IllegalStateException("No valid configuration available", lastError)

        return verifyEnvelope(cached,state.highestConfigVersion,now)
    }

    private fun accept(raw:String,minimum:Long,now:Instant):VerifiedConfig {
        val verified=verifyEnvelope(raw,minimum,now)
        state.configEnvelope=raw
        state.highestConfigVersion=maxOf(minimum,verified.version)
        state.configMirrorUrls=extractMirrorUrls(verified.payload)
        return verified
    }

    private fun verifyEnvelope(raw:String,minimum:Long,now:Instant):VerifiedConfig {
        val trust=trustRepository?.refreshOrFallback(now)
        if(trust!=null){
            return ConfigVerifier.verifyWithKeys(
                raw,minimum,now,trust.verificationKeys("config")
            )
        }
        return requireNotNull(legacyVerifier){"No configuration trust source"}.verify(raw,minimum,now)
    }

    private fun extractMirrorUrls(payload:JSONObject):List<String> {
        val nodes=payload.optJSONArray("config_mirrors") ?: return emptyList()
        val out=mutableListOf<Pair<Int,String>>()
        for(i in 0 until nodes.length()){
            val node=nodes.optJSONObject(i) ?: continue
            val endpoints=node.optJSONArray("endpoints") ?: continue
            for(j in 0 until endpoints.length()){
                val ep=endpoints.optJSONObject(j) ?: continue
                if(ep.optString("kind")!="config_mirror" ||
                    ep.optString("scheme")!="https" ||
                    ep.optString("transport")!="https") continue
                val host=ep.optString("host").trim()
                val port=ep.optInt("port",0)
                if(host.isEmpty() || port !in 1..65535) continue
                val path=ep.optString("path","").trim().ifEmpty { "/api/v1/config/latest" }
                if(!path.startsWith("/")) continue
                out += ep.optInt("priority",100) to "https://$host:$port$path"
            }
        }
        return out.sortedBy{it.first}.map{it.second}.distinct().take(16)
    }
}
