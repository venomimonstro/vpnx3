package ru.vpnx3.app.update

import ru.vpnx3.app.BuildConfig
import java.net.HttpURLConnection
import java.net.URL

class UpdateRepository {
    fun latest(): ReleaseInfo? {
        if (BuildConfig.RELEASE_PUBLIC_KEY.isBlank()) return null
        val c=URL(BuildConfig.CONTROL_URL.trimEnd('/')+"/api/v1/releases/latest?target=android_apk")
            .openConnection() as HttpURLConnection
        c.requestMethod="GET";c.connectTimeout=5000;c.readTimeout=7000
        val code=c.responseCode
        if(code==404||code==503){c.disconnect();return null}
        val raw=(if(code in 200..299)c.inputStream else c.errorStream)
            ?.bufferedReader()?.use{it.readText()}.orEmpty()
        c.disconnect()
        if(code !in 200..299) return null
        val info=ReleaseVerifier(BuildConfig.RELEASE_PUBLIC_KEY).verify(raw)
        if(info.version==BuildConfig.VERSION_NAME) return null
        return info
    }
}
