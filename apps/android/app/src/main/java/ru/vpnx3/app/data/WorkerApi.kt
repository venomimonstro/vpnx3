package ru.vpnx3.app.data

import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

data class WorkerSession(
    val sessionId: String,
    val assignedIp: String,
    val serverPublicKey: String,
    val endpoint: String,
    val expiresAt: String
)

class WorkerApi {
    fun createSession(
        endpoint: NetworkEndpoint,
        leaseEnvelope: String,
        clientPublicKey: String
    ): WorkerSession {
        val body = JSONObject()
            .put("lease", JSONObject(leaseEnvelope))
            .put("client_public_key", clientPublicKey)
            .toString()

        val response = request(
            url = endpoint.httpsUrl(),
            method = "POST",
            body = body
        )
        val json = JSONObject(response)
        val config = json.getJSONObject("config")
        return WorkerSession(
            sessionId = json.getString("id"),
            assignedIp = config.getString("assigned_ip"),
            serverPublicKey = config.getString("server_public_key"),
            endpoint = config.getString("endpoint"),
            expiresAt = config.getString("expires_at")
        )
    }

    fun closeSession(endpoint: NetworkEndpoint, sessionId: String) {
        val base = endpoint.httpsUrl().trimEnd('/')
        request(
            url = "$base/$sessionId",
            method = "DELETE",
            body = null
        )
    }

    private fun request(url: String, method: String, body: String?): String {
        val connection = URL(url).openConnection() as HttpURLConnection
        connection.requestMethod = method
        connection.connectTimeout = 8_000
        connection.readTimeout = 10_000
        connection.setRequestProperty("Accept", "application/json")

        if (body != null) {
            connection.doOutput = true
            connection.setRequestProperty("Content-Type", "application/json")
            connection.outputStream.use {
                it.write(body.toByteArray(Charsets.UTF_8))
            }
        }

        val code = connection.responseCode
        val stream = if (code in 200..299) connection.inputStream else connection.errorStream
        val response = stream?.bufferedReader()?.use { it.readText() }.orEmpty()
        connection.disconnect()

        if (code !in 200..299) {
            throw IllegalStateException("Worker API returned HTTP $code")
        }
        return response
    }
}
