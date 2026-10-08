package ru.vpnx3.app.security

import com.google.crypto.tink.subtle.Ed25519Verify
import org.json.JSONObject
import java.time.Instant
import java.util.Base64

data class VerifiedConfig(
    val version: Long,
    val expiresAt: Instant,
    val payload: JSONObject
)

class ConfigVerifier(private val trust:VerifiedTrustBundle) {
    fun verify(rawEnvelope: String, minimumVersion: Long, now: Instant): VerifiedConfig {
        val envelope = JSONObject(rawEnvelope)
        val envelopeKeyId = envelope.getString("key_id")
        val trusted=trust.key("config",envelopeKeyId)
            ?: throw IllegalArgumentException("Configuration signing key is not authorized by trust bundle")
        val publicKey=Base64.getUrlDecoder().decode(trusted.publicKey)
        val payload = Base64.getUrlDecoder().decode(envelope.getString("payload"))
        val signature = Base64.getUrlDecoder().decode(envelope.getString("signature"))
        Ed25519Verify(publicKey).verify(signature, payload)

        val json = JSONObject(payload.toString(Charsets.UTF_8))
        require(json.getInt("schema_version") == 1) { "Unsupported configuration schema" }
        val version = json.getLong("version")
        require(version > 0) { "Invalid configuration version" }
        require(version >= minimumVersion) { "Configuration rollback detected" }
        val createdAt = Instant.parse(json.getString("created_at"))
        val expiresAt = Instant.parse(json.getString("expires_at"))
        require(!createdAt.isAfter(now.plusSeconds(300))) { "Configuration is from the future" }
        require(expiresAt.isAfter(now)) { "Configuration has expired" }

        return VerifiedConfig(version, expiresAt, json)
    }
}
