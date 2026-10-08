package ru.vpnx3.app.update

import com.google.crypto.tink.subtle.Ed25519Verify
import org.json.JSONObject
import ru.vpnx3.app.security.VerifiedTrustBundle
import java.time.Instant
import java.util.Base64

data class ReleaseInfo(
    val version: String,
    val artifactId: String,
    val sha256: String,
    val sizeBytes: Long,
    val downloadPath: String
)

class ReleaseVerifier(private val trust:VerifiedTrustBundle) {
    fun verify(rawEnvelope:String): ReleaseInfo {
        val env=JSONObject(rawEnvelope)
        val keyId=env.getString("key_id")
        val trusted=trust.key("release",keyId)
            ?: throw IllegalArgumentException("Release signing key is not authorized")
        val payload=Base64.getUrlDecoder().decode(env.getString("payload"))
        val signature=Base64.getUrlDecoder().decode(env.getString("signature"))
        Ed25519Verify(Base64.getUrlDecoder().decode(trusted.publicKey)).verify(signature,payload)
        val json=JSONObject(payload.toString(Charsets.UTF_8))
        require(json.getInt("schema_version")==1)
        require(json.getString("target")=="android_apk")
        val issuedAt=Instant.parse(json.getString("issued_at"))
        val expiresAt=Instant.parse(json.getString("expires_at"))
        val now=Instant.now()
        require(!issuedAt.isAfter(now.plusSeconds(300)))
        require(expiresAt.isAfter(now))
        val sha=json.getString("sha256")
        require(sha.matches(Regex("^[0-9a-f]{64}$")))
        return ReleaseInfo(
            json.getString("version"),json.getString("artifact_id"),sha,
            json.getLong("size_bytes"),json.getString("download_path")
        )
    }
}
