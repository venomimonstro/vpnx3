package ru.vpnx3.app.update

import com.google.crypto.tink.subtle.Ed25519Verify
import org.json.JSONObject
import ru.vpnx3.app.security.VerifiedTrustBundle
import java.time.Instant
import java.util.Base64

data class ReleasePolicy(
    val minimumSupportedVersion:String,
    val recommendedVersion:String,
    val rolloutPercent:Int,
    val blockedVersions:Set<String>,
    val message:String
)

class ReleasePolicyVerifier(private val trust:VerifiedTrustBundle) {
    fun verify(rawEnvelope:String):ReleasePolicy {
        val env=JSONObject(rawEnvelope)
        val trusted=trust.key("release",env.getString("key_id"))
            ?: throw IllegalArgumentException("Release signing key is not authorized")
        val payload=Base64.getUrlDecoder().decode(env.getString("payload"))
        val signature=Base64.getUrlDecoder().decode(env.getString("signature"))
        Ed25519Verify(Base64.getUrlDecoder().decode(trusted.publicKey)).verify(signature,payload)
        val json=JSONObject(payload.toString(Charsets.UTF_8))
        require(json.getInt("schema_version")==1)
        require(json.getString("target")=="android_apk")
        val now=Instant.now()
        val issued=Instant.parse(json.getString("issued_at"))
        val expires=Instant.parse(json.getString("expires_at"))
        require(!issued.isAfter(now.plusSeconds(300)))
        require(expires.isAfter(now))
        val blocked=json.getJSONArray("blocked_versions")
        val set=buildSet { for(i in 0 until blocked.length()) add(blocked.getString(i)) }
        val rollout=json.getInt("rollout_percent");require(rollout in 0..100)
        return ReleasePolicy(
            json.optString("minimum_supported_version",""),
            json.optString("recommended_version",""),
            rollout,set,json.optString("message","")
        )
    }
}
