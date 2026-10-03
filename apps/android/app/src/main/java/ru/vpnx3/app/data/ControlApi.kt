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

data class ClientPlan(
    val id: String,
    val code: String,
    val version: Int,
    val name: String,
    val priceMinor: Long,
    val currency: String,
    val billingPeriodDays: Int,
    val deviceLimit: Int
)

data class PaymentStart(
    val paymentId: String,
    val confirmationUrl: String
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

    fun lease(deviceId: String, sequence: Long, tunnelPublicKey: String): AccessLease {
        val path = "/api/v1/client/lease"
        val body = JSONObject()
            .put("sequence", sequence)
            .put("tunnel_public_key", tunnelPublicKey)
            .toString()
            .toByteArray(Charsets.UTF_8)

        return AccessLease(
            rawEnvelope = signedRequest("POST", path, body, deviceId)
        )
    }

    fun latestConfig(): String =
        request("GET", "/api/v1/config/latest", null, emptyMap())

    fun plans(): List<ClientPlan> {
        val raw=request("GET","/api/v1/plans",null,emptyMap())
        val arr=JSONObject(raw).getJSONArray("plans")
        return buildList {
            for(i in 0 until arr.length()){
                val p=arr.getJSONObject(i)
                add(ClientPlan(
                    id=p.getString("id"),
                    code=p.getString("code"),
                    version=p.getInt("version"),
                    name=p.getString("name"),
                    priceMinor=p.getLong("price_minor"),
                    currency=p.getString("currency"),
                    billingPeriodDays=p.getInt("billing_period_days"),
                    deviceLimit=p.getInt("device_limit")
                ))
            }
        }
    }

    fun telemetry(
        deviceId:String,
        sequence:Long,
        eventType:String,
        configVersion:Long,
        workerNodeId:String?,
        networkType:String,
        durationMS:Long
    ) {
        val path="/api/v1/client/telemetry"
        val body=JSONObject()
            .put("sequence",sequence)
            .put("platform","android")
            .put("client_version",ru.vpnx3.app.BuildConfig.VERSION_NAME)
            .put("event_type",eventType)
            .put("config_version",configVersion)
            .put("worker_node_id",workerNodeId ?: "")
            .put("network_type",networkType)
            .put("duration_ms",durationMS.coerceAtLeast(0))
            .toString()
            .toByteArray(Charsets.UTF_8)
        signedRequest("POST",path,body,deviceId)
    }

    fun createPayment(deviceId:String,sequence:Long,planId:String): PaymentStart {
        val path="/api/v1/client/payments"
        val body=JSONObject()
            .put("sequence",sequence)
            .put("plan_id",planId)
            .toString()
            .toByteArray(Charsets.UTF_8)
        val raw=signedRequest("POST",path,body,deviceId)
        val json=JSONObject(raw)
        return PaymentStart(
            paymentId=json.getString("payment_id"),
            confirmationUrl=json.getString("confirmation_url")
        )
    }

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
