package ru.vpnx3.app.update

import com.google.crypto.tink.subtle.Ed25519Verify
import org.json.JSONObject
import java.security.MessageDigest
import java.time.Instant
import java.util.Base64

data class ReleaseInfo(
    val version: String,
    val artifactId: String,
    val sha256: String,
    val sizeBytes: Long,
    val downloadPath: String
)

class ReleaseVerifier(publicKeyBase64Url: String) {
    private val publicKey = Base64.getUrlDecoder().decode(publicKeyBase64Url)
    private val verifier = Ed25519Verify(publicKey)
    private val keyId = MessageDigest.getInstance("SHA-256")
        .digest(publicKey).take(8).joinToString("") { "%02x".format(it) }

    init { require(publicKey.size==32) { "Release public key must contain 32 bytes" } }

    fun verify(rawEnvelope:String): ReleaseInfo {
        val env=JSONObject(rawEnvelope)
        require(env.getString("key_id")==keyId) { "Unexpected release signing key" }
        val payload=Base64.getUrlDecoder().decode(env.getString("payload"))
        val signature=Base64.getUrlDecoder().decode(env.getString("signature"))
        verifier.verify(signature,payload)
        val json=JSONObject(payload.toString(Charsets.UTF_8))
        require(json.getInt("schema_version")==1)
        require(json.getString("target")=="android_apk")
        val issuedAt=Instant.parse(json.getString("issued_at"))
        val expiresAt=Instant.parse(json.getString("expires_at"))
        val now=Instant.now()
        require(!issuedAt.isAfter(now.plusSeconds(300))) { "Release metadata is from the future" }
        require(expiresAt.isAfter(now)) { "Release metadata expired" }
        val sha=json.getString("sha256")
        require(sha.matches(Regex("^[0-9a-f]{64}$")))
        return ReleaseInfo(
            version=json.getString("version"),
            artifactId=json.getString("artifact_id"),
            sha256=sha,
            sizeBytes=json.getLong("size_bytes"),
            downloadPath=json.getString("download_path")
        )
    }
}
