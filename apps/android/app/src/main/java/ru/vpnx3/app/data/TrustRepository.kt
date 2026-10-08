package ru.vpnx3.app.data

import ru.vpnx3.app.security.TrustBundleVerifier
import ru.vpnx3.app.security.VerifiedTrustBundle
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant

class TrustRepository(
    private val api:ControlApi,
    private val state:LocalState,
    rootPublicKey:String,
    bootstrapUrls:String=""
) {
    private val verifier=TrustBundleVerifier(rootPublicKey)
    private val bootstrap=bootstrapUrls.split(',', ';')
        .map{it.trim()}
        .filter{it.startsWith("https://")}
        .distinct()
        .take(16)

    fun refreshOrFallback(now:Instant=Instant.now()):VerifiedTrustBundle {
        val minimum=state.highestTrustBundleVersion
        var lastError:Throwable?=null

        val primary=runCatching{api.trustBundle()}
            .mapCatching{accept(it,minimum,now)}
        if(primary.isSuccess)return primary.getOrThrow()
        lastError=primary.exceptionOrNull()

        val mirrors=(bootstrap+state.configMirrorUrls).distinct().take(32)
        for(url in mirrors){
            val result=runCatching{trustFromMirror(url)}
                .mapCatching{accept(it,minimum,now)}
            if(result.isSuccess)return result.getOrThrow()
            lastError=result.exceptionOrNull() ?: lastError
        }

        val cached=state.trustBundleEnvelope
            ?: throw IllegalStateException("No valid trust bundle available",lastError)
        return verifier.verify(cached,state.highestTrustBundleVersion,now)
    }

    private fun accept(raw:String,minimum:Long,now:Instant):VerifiedTrustBundle{
        val verified=verifier.verify(raw,minimum,now)
        state.trustBundleEnvelope=raw
        state.highestTrustBundleVersion=maxOf(minimum,verified.version)
        return verified
    }

    private fun trustFromMirror(configUrl:String):String{
        val source=URL(configUrl)
        require(source.protocol=="https"&&source.host.isNotBlank()&&source.userInfo==null)
        val url=URL(source.protocol,source.host,source.port,"/api/v1/trust/bundle")
        val c=url.openConnection() as HttpURLConnection
        c.requestMethod="GET";c.connectTimeout=8_000;c.readTimeout=10_000
        c.setRequestProperty("Accept","application/json")
        c.setRequestProperty("Cache-Control","no-cache")
        val code=c.responseCode
        val raw=(if(code in 200..299)c.inputStream else c.errorStream)
            ?.bufferedReader()?.use{it.readText()}.orEmpty()
        c.disconnect()
        if(code !in 200..299)throw IllegalStateException("Trust mirror failed with HTTP $code")
        return raw
    }
}
