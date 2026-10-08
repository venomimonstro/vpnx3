package ru.vpnx3.app.update

import ru.vpnx3.app.BuildConfig
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest

data class UpdateDecision(
    val availableVersion:String?=null,
    val required:Boolean=false,
    val blocked:Boolean=false,
    val message:String?=null
)

class UpdateRepository {
    fun evaluate(deviceId:String):UpdateDecision {
        if(BuildConfig.RELEASE_PUBLIC_KEY.isBlank()) return UpdateDecision()

        val policyRaw=get("/api/v1/releases/policy?target=android_apk") ?: return fallbackLatest()
        val policy=ReleasePolicyVerifier(BuildConfig.RELEASE_PUBLIC_KEY).verify(policyRaw)
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

    private fun fallbackLatest():UpdateDecision {
        val info=latest() ?: return UpdateDecision()
        return UpdateDecision(availableVersion=info.version)
    }

    fun latest():ReleaseInfo? {
        val raw=get("/api/v1/releases/latest?target=android_apk") ?: return null
        val info=ReleaseVerifier(BuildConfig.RELEASE_PUBLIC_KEY).verify(raw)
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
