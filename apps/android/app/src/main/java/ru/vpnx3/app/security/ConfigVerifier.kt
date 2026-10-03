package ru.vpnx3.app.security

import com.google.crypto.tink.subtle.Ed25519Verify
import org.json.JSONObject
import java.security.MessageDigest
import java.time.Instant
import java.util.Base64

data class VerifiedConfig(
    val version: Long,
    val expiresAt: Instant,
    val payload: JSONObject
)

class ConfigVerifier(publicKeyBase64Url: String) {
    private val publicKey = Base64.getUrlDecoder().decode(publicKeyBase64Url)
    private val verifier = Ed25519Verify(publicKey)
    private val keyId: String = MessageDigest.getInstance("SHA-256")
        .digest(publicKey)
        .take(8)
        .joinToString("") { "%02x".format(it) }

    init {
        require(publicKey.size == 32) { "Configuration public key must contain 32 bytes" }
    }

    fun verify(rawEnvelope: String, minimumVersion: Long, now: Instant): VerifiedConfig {
        val envelope = JSONObject(rawEnvelope)
        val envelopeKeyId = envelope.getString("key_id")
        require(envelopeKeyId == keyId) { "Unexpected configuration signing key" }

        val payload = Base64.getUrlDecoder().decode(envelope.getString("payload"))
        val signature = Base64.getUrlDecoder().decode(envelope.getString("signature"))
        verifier.verify(signature, payload)

        val json = JSONObject(payload.toString(Charsets.UTF_8))
        require(json.getInt("schema_version") == 1) { "Unsupported configuration schema" }

        val version = json.getLong("version")
        require(version > 0) { "Invalid configuration version" }
        require(version >= minimumVersion) { "Configuration rollback detected" }

        val createdAt = Instant.parse(json.getString("created_at"))
        val expiresAt = Instant.parse(json.getString("expires_at"))
        require(!createdAt.isAfter(now.plusSeconds(300))) { "Configuration is from the future" }
        require(expiresAt.isAfter(now)) { "Configuration has expired" }

        return VerifiedConfig(
            version = version,
            expiresAt = expiresAt,
            payload = json
        )
    }
}
