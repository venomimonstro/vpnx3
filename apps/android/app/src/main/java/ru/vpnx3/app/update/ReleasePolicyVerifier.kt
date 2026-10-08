package ru.vpnx3.app.update

import com.google.crypto.tink.subtle.Ed25519Verify
import org.json.JSONObject
import java.security.MessageDigest
import java.time.Instant
import java.util.Base64

data class ReleasePolicy(
    val minimumSupportedVersion:String,
    val recommendedVersion:String,
    val rolloutPercent:Int,
    val blockedVersions:Set<String>,
    val message:String
)

class ReleasePolicyVerifier(publicKeyBase64Url:String) {
    private val publicKey=Base64.getUrlDecoder().decode(publicKeyBase64Url)
    private val verifier=Ed25519Verify(publicKey)
    private val keyId=MessageDigest.getInstance("SHA-256")
        .digest(publicKey).take(8).joinToString(""){"%02x".format(it)}

    init { require(publicKey.size==32) }

    fun verify(rawEnvelope:String):ReleasePolicy {
        val env=JSONObject(rawEnvelope)
        require(env.getString("key_id")==keyId)
        val payload=Base64.getUrlDecoder().decode(env.getString("payload"))
        val signature=Base64.getUrlDecoder().decode(env.getString("signature"))
        verifier.verify(signature,payload)
        val json=JSONObject(payload.toString(Charsets.UTF_8))
        require(json.getInt("schema_version")==1)
        require(json.getString("target")=="android_apk")

        val now=Instant.now()
        val issued=Instant.parse(json.getString("issued_at"))
        val expires=Instant.parse(json.getString("expires_at"))
        require(!issued.isAfter(now.plusSeconds(300)))
        require(expires.isAfter(now))

        val blocked=json.getJSONArray("blocked_versions")
        val set=buildSet {
            for(i in 0 until blocked.length()) add(blocked.getString(i))
        }
        val rollout=json.getInt("rollout_percent")
        require(rollout in 0..100)
        return ReleasePolicy(
            minimumSupportedVersion=json.optString("minimum_supported_version",""),
            recommendedVersion=json.optString("recommended_version",""),
            rolloutPercent=rollout,
            blockedVersions=set,
            message=json.optString("message","")
        )
    }
}
