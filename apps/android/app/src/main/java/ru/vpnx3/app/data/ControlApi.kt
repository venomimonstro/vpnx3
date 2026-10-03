package ru.vpnx3.app.data

import org.json.JSONObject
import ru.vpnx3.app.security.DeviceIdentity
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant

data class Registration(
    val userId: String,
    val deviceId: String,
    val trialExpiresAt: String
)

data class AccessLease(
    val rawEnvelope: String
)

class ControlApi(
    private val baseUrl: String,
    private val identity: DeviceIdentity
) {
    fun register(displayName: String): Registration {
        val path = "/api/v1/client/register"
        val body = JSONObject()
            .put("platform", "android")
            .put("display_name", displayName)
            .put("identity_algorithm", DeviceIdentity.ALGORITHM)
            .put("public_key", identity.publicKeyBase64Url())
            .toString()
            .toByteArray(Charsets.UTF_8)

        val response = signedRequest("POST", path, body, null)
        val json = JSONObject(response)
        return Registration(
            userId = json.getString("user_id"),
            deviceId = json.getString("device_id"),
            trialExpiresAt = json.getString("trial_expires_at")
        )
    }

    fun lease(deviceId: String, sequence: Long): AccessLease {
        val path = "/api/v1/client/lease"
        val body = JSONObject()
            .put("sequence", sequence)
            .toString()
            .toByteArray(Charsets.UTF_8)

        return AccessLease(
            rawEnvelope = signedRequest("POST", path, body, deviceId)
        )
    }

    fun latestConfig(): String =
        request("GET", "/api/v1/config/latest", null, emptyMap())

    private fun signedRequest(
        method: String,
        path: String,
        body: ByteArray,
        deviceId: String?
    ): String {
        val timestamp = Instant.now().epochSecond.toString()
        val headers = linkedMapOf(
            "X-VPNX3-Timestamp" to timestamp,
            "X-VPNX3-Signature" to identity.sign(method, path, timestamp, body)
        )
        if (deviceId != null) headers["X-VPNX3-Device-ID"] = deviceId
        return request(method, path, body, headers)
    }

    private fun request(
        method: String,
        path: String,
        body: ByteArray?,
        headers: Map<String, String>
    ): String {
        val connection = URL(baseUrl.trimEnd('/') + path)
            .openConnection() as HttpURLConnection
        connection.requestMethod = method
        connection.connectTimeout = 8_000
        connection.readTimeout = 10_000
        connection.setRequestProperty("Accept", "application/json")
        headers.forEach { (key, value) ->
            connection.setRequestProperty(key, value)
        }

        if (body != null) {
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            connection.outputStream.use { it.write(body) }
        }

        val code = connection.responseCode
        val stream = if (code in 200..299) {
            connection.inputStream
        } else {
            connection.errorStream
        }
        val response = stream?.bufferedReader()?.use { it.readText() }.orEmpty()
        connection.disconnect()

        if (code !in 200..299) {
            throw IllegalStateException("Control API request failed with HTTP $code")
        }
        return response
    }
}
