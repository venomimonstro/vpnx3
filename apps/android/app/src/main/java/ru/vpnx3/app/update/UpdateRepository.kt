package ru.vpnx3.app.update

import android.content.Context
import ru.vpnx3.app.BuildConfig
import ru.vpnx3.app.data.ControlApi
import ru.vpnx3.app.data.LocalState
import ru.vpnx3.app.data.TrustRepository
import ru.vpnx3.app.security.DeviceIdentity
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest

data class UpdateDecision(
    val availableVersion:String?=null,
    val required:Boolean=false,
    val blocked:Boolean=false,
    val message:String?=null
)

class UpdateRepository(context:Context) {
    private val trustRepository:TrustRepository? =
        BuildConfig.TRUST_ROOT_PUBLIC_KEY.takeIf{it.isNotBlank()}?.let{
            TrustRepository(
                ControlApi(BuildConfig.CONTROL_URL,DeviceIdentity()),
                LocalState(context.applicationContext),
                it
            )
        }

    private fun releaseKeys():Map<String,String> {
        val trust=trustRepository?.refreshOrFallback()
        if(trust!=null) return trust.verificationKeys("release")
        val legacy=BuildConfig.RELEASE_PUBLIC_KEY
        if(legacy.isBlank()) return emptyMap()
        val raw=java.util.Base64.getUrlDecoder().decode(legacy)
        val id=MessageDigest.getInstance("SHA-256").digest(raw)
            .take(8).joinToString(""){"%02x".format(it)}
        return mapOf(id to legacy)
    }
    fun evaluate(deviceId:String):UpdateDecision {
        val keys=runCatching{releaseKeys()}.getOrDefault(emptyMap())
        if(keys.isEmpty()) return UpdateDecision()

        val policyRaw=get("/api/v1/releases/policy?target=android_apk") ?: return fallbackLatest(keys)
        val policy=ReleasePolicyVerifier.verifyWithKeys(policyRaw,keys)
        val current=BuildConfig.VERSION_NAME

        val blocked=current in policy.blockedVersions
        val belowMinimum=policy.minimumSupportedVersion.isNotBlank() &&
            compareVersions(current,policy.minimumSupportedVersion)<0

        if(blocked||belowMinimum){
            return UpdateDecision(
                availableVersion=policy.recommendedVersion.ifBlank{null},
                required=true,
                blocked=blocked,
                message=policy.message.ifBlank{
                    if(blocked) "Эта версия приложения отключена. Установите обновление."
                    else "Для продолжения работы требуется обновить VPNX3."
                }
            )
        }

        val recommended=policy.recommendedVersion
        if(recommended.isBlank()||compareVersions(current,recommended)>=0) return UpdateDecision()

        val cohort=stableCohort(deviceId,"android_apk",recommended)
        return if(cohort<policy.rolloutPercent) {
            UpdateDecision(
                availableVersion=recommended,
                message=policy.message.ifBlank{null}
            )
        } else UpdateDecision()
    }

    private fun fallbackLatest(keys:Map<String,String>):UpdateDecision {
        val info=latest(keys) ?: return UpdateDecision()
        return UpdateDecision(availableVersion=info.version)
    }

    fun latest(keys:Map<String,String> = runCatching{releaseKeys()}.getOrDefault(emptyMap())):ReleaseInfo? {
        if(keys.isEmpty()) return null
        val raw=get("/api/v1/releases/latest?target=android_apk") ?: return null
        val info=ReleaseVerifier.verifyWithKeys(raw,keys)
        if(info.version==BuildConfig.VERSION_NAME) return null
        return info
    }

    private fun get(path:String):String? {
        val c=URL(BuildConfig.CONTROL_URL.trimEnd('/')+path).openConnection() as HttpURLConnection
        c.requestMethod="GET";c.connectTimeout=5000;c.readTimeout=7000
        val code=c.responseCode
        if(code==404||code==503){c.disconnect();return null}
        val raw=(if(code in 200..299)c.inputStream else c.errorStream)
            ?.bufferedReader()?.use{it.readText()}.orEmpty()
        c.disconnect()
        if(code !in 200..299) return null
        return raw
    }

    private fun stableCohort(deviceId:String,target:String,version:String):Int {
        val digest=MessageDigest.getInstance("SHA-256")
            .digest("$deviceId\u0000$target\u0000$version".toByteArray(Charsets.UTF_8))
        val value=((digest[0].toInt() and 0xff) shl 8) or (digest[1].toInt() and 0xff)
        return value % 100
    }

    private fun compareVersions(a:String,b:String):Int {
        val aa=parseVersion(a);val bb=parseVersion(b)
        val n=maxOf(aa.size,bb.size)
        for(i in 0 until n){
            val av=aa.getOrElse(i){0};val bv=bb.getOrElse(i){0}
            if(av!=bv)return av.compareTo(bv)
        }
        return 0
    }

    private fun parseVersion(value:String):List<Int> =
        value.trim().removePrefix("v").substringBefore('-')
            .split('.').map{it.toIntOrNull() ?: 0}
}
