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

data class ClientAccountStatus(
    val userId: String,
    val entitlement: String,
    val planCode: String?,
    val planName: String?,
    val expiresAt: String,
    val graceUntil: String?,
    val deviceLimit: Int,
    val activeDevices: Int,
    val autoRenew: Boolean
)

data class PairingCode(
    val code: String,
    val expiresAt: String
)

data class ClientDevice(
    val id:String,
    val platform:String,
    val displayName:String,
    val status:String,
    val clientVersion:String?,
    val firstSeenAt:String,
    val lastSeenAt:String?,
    val current:Boolean
)

data class ReferralStatus(
    val code: String?,
    val claimed30d: Long,
    val qualified30d: Long,
    val rewardDaysGranted: Long,
    val referredBy: String?
)

data class ReferralClaimResult(
    val rewardDays: Int,
    val rewardApplied: Boolean,
    val referrerRewardPending: Boolean
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

    fun latestConfigFrom(absoluteUrl:String):String {
        val url=URL(absoluteUrl)
        require(url.protocol=="https" && url.host.isNotBlank() && url.userInfo==null) {
            "Configuration mirror must use credential-free HTTPS"
        }
        require(url.ref==null) { "Configuration mirror URL must not contain fragment" }
        val connection=url.openConnection() as HttpURLConnection
        connection.requestMethod="GET"
        connection.connectTimeout=8_000
        connection.readTimeout=10_000
        connection.setRequestProperty("Accept","application/json")
        connection.setRequestProperty("Cache-Control","no-cache")
        val code=connection.responseCode
        val stream=if(code in 200..299) connection.inputStream else connection.errorStream
        val response=stream?.bufferedReader()?.use{it.readText()}.orEmpty()
        connection.disconnect()
        if(code !in 200..299) throw IllegalStateException("Config mirror failed with HTTP $code")
        return response
    }

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

    fun accountStatus(deviceId:String,sequence:Long): ClientAccountStatus {
        val path="/api/v1/client/account/status"
        val body=JSONObject().put("sequence",sequence).toString().toByteArray(Charsets.UTF_8)
        return parseAccountStatus(JSONObject(signedRequest("POST",path,body,deviceId)))
    }

    fun createPairingCode(deviceId:String,sequence:Long): PairingCode {
        val path="/api/v1/client/pairing-code"
        val body=JSONObject().put("sequence",sequence).toString().toByteArray(Charsets.UTF_8)
        val json=JSONObject(signedRequest("POST",path,body,deviceId))
        return PairingCode(json.getString("code"),json.getString("expires_at"))
    }

    fun claimPairingCode(deviceId:String,sequence:Long,code:String): ClientAccountStatus {
        val path="/api/v1/client/pairing-claim"
        val body=JSONObject().put("sequence",sequence).put("code",code).toString().toByteArray(Charsets.UTF_8)
        return parseAccountStatus(JSONObject(signedRequest("POST",path,body,deviceId)))
    }

    fun referralCode(deviceId:String,sequence:Long): String {
        val path="/api/v1/client/referral/code"
        val body=JSONObject().put("sequence",sequence).toString().toByteArray(Charsets.UTF_8)
        return JSONObject(signedRequest("POST",path,body,deviceId)).getString("code")
    }

    fun referralStatus(deviceId:String,sequence:Long): ReferralStatus {
        val path="/api/v1/client/referral/status"
        val body=JSONObject().put("sequence",sequence).toString().toByteArray(Charsets.UTF_8)
        val json=JSONObject(signedRequest("POST",path,body,deviceId))
        return ReferralStatus(
            code=json.optString("code").takeIf{it.isNotBlank()},
            claimed30d=json.optLong("claimed_30d",0),
            qualified30d=json.optLong("qualified_30d",0),
            rewardDaysGranted=json.optLong("reward_days_granted",0),
            referredBy=json.optString("referred_by").takeIf{it.isNotBlank()}
        )
    }

    fun claimReferralCode(deviceId:String,sequence:Long,code:String): ReferralClaimResult {
        val path="/api/v1/client/referral/claim"
        val body=JSONObject()
            .put("sequence",sequence)
            .put("code",code)
            .toString()
            .toByteArray(Charsets.UTF_8)
        val json=JSONObject(signedRequest("POST",path,body,deviceId))
        return ReferralClaimResult(
            rewardDays=json.getInt("reward_days"),
            rewardApplied=json.optBoolean("reward_applied",false),
            referrerRewardPending=json.optBoolean("referrer_reward_pending",true)
        )
    }

    fun devices(deviceId:String,sequence:Long):List<ClientDevice>{
        val path="/api/v1/client/devices"
        val body=JSONObject().put("sequence",sequence).toString().toByteArray(Charsets.UTF_8)
        val arr=JSONObject(signedRequest("POST",path,body,deviceId)).getJSONArray("devices")
        return buildList {
            for(i in 0 until arr.length()){
                val d=arr.getJSONObject(i)
                add(ClientDevice(
                    id=d.getString("id"),
                    platform=d.getString("platform"),
                    displayName=d.getString("display_name"),
                    status=d.getString("status"),
                    clientVersion=d.optString("client_version").takeIf{it.isNotBlank()},
                    firstSeenAt=d.getString("first_seen_at"),
                    lastSeenAt=d.optString("last_seen_at").takeIf{it.isNotBlank()},
                    current=d.optBoolean("current",false)
                ))
            }
        }
    }

    fun revokeDevice(deviceId:String,sequence:Long,targetDeviceId:String){
        val path="/api/v1/client/devices/revoke"
        val body=JSONObject()
            .put("sequence",sequence)
            .put("device_id",targetDeviceId)
            .toString().toByteArray(Charsets.UTF_8)
        signedRequest("POST",path,body,deviceId)
    }

    fun setAutoRenew(deviceId:String,sequence:Long,enabled:Boolean): ClientAccountStatus {
        val path="/api/v1/client/account/auto-renew"
        val body=JSONObject()
            .put("sequence",sequence)
            .put("enabled",enabled)
            .toString()
            .toByteArray(Charsets.UTF_8)
        return parseAccountStatus(JSONObject(signedRequest("POST",path,body,deviceId)))
    }

    private fun parseAccountStatus(json:JSONObject): ClientAccountStatus = ClientAccountStatus(
        userId=json.getString("user_id"),
        entitlement=json.getString("entitlement"),
        planCode=json.optString("plan_code").takeIf{it.isNotBlank()},
        planName=json.optString("plan_name").takeIf{it.isNotBlank()},
        expiresAt=json.getString("expires_at"),
        graceUntil=json.optString("grace_until").takeIf{it.isNotBlank()},
        deviceLimit=json.getInt("device_limit"),
        activeDevices=json.getInt("active_devices"),
        autoRenew=json.optBoolean("auto_renew",false)
    )

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

    fun createPayment(deviceId:String,sequence:Long,planId:String,autoRenew:Boolean): PaymentStart {
        val path="/api/v1/client/payments"
        val body=JSONObject()
            .put("sequence",sequence)
            .put("plan_id",planId)
            .put("auto_renew",autoRenew)
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
