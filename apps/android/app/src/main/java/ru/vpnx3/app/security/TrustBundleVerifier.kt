package ru.vpnx3.app.security

import com.google.crypto.tink.subtle.Ed25519Verify
import org.json.JSONObject
import java.security.MessageDigest
import java.time.Instant
import java.util.Base64

data class TrustedKey(
    val purpose:String,
    val keyId:String,
    val publicKey:String,
    val state:String
)

data class VerifiedTrustBundle(
    val version:Long,
    val expiresAt:Instant,
    val keys:List<TrustedKey>
) {
    fun key(purpose:String,keyId:String):TrustedKey? =
        keys.firstOrNull { it.purpose==purpose && it.keyId==keyId && it.state in setOf("active","next","retired") }

    fun active(purpose:String):TrustedKey? =
        keys.firstOrNull { it.purpose==purpose && it.state=="active" }
}

class TrustBundleVerifier(rootPublicKeyBase64Url:String) {
    private val root=Base64.getUrlDecoder().decode(rootPublicKeyBase64Url)
    private val verifier=Ed25519Verify(root)
    private val rootKeyId=MessageDigest.getInstance("SHA-256")
        .digest(root).take(8).joinToString(""){"%02x".format(it)}

    init { require(root.size==32) { "Trust root public key must contain 32 bytes" } }

    fun verify(rawEnvelope:String,minimumVersion:Long,now:Instant):VerifiedTrustBundle {
        val env=JSONObject(rawEnvelope)
        require(env.getString("key_id")==rootKeyId) { "Unexpected trust root key" }
        val payload=Base64.getUrlDecoder().decode(env.getString("payload"))
        val signature=Base64.getUrlDecoder().decode(env.getString("signature"))
        verifier.verify(signature,payload)

        val json=JSONObject(payload.toString(Charsets.UTF_8))
        require(json.getInt("schema_version")==1) { "Unsupported trust bundle schema" }
        val version=json.getLong("version")
        require(version>0 && version>=minimumVersion) { "Trust bundle rollback detected" }
        val issuedAt=Instant.parse(json.getString("issued_at"))
        val expiresAt=Instant.parse(json.getString("expires_at"))
        require(!issuedAt.isAfter(now.plusSeconds(300))) { "Trust bundle is from the future" }
        require(expiresAt.isAfter(now)) { "Trust bundle expired" }

        val arr=json.getJSONArray("keys")
        require(arr.length() in 2..12) { "Invalid trust key count" }
        val keys=buildList {
            for(i in 0 until arr.length()) {
                val k=arr.getJSONObject(i)
                val purpose=k.getString("purpose")
                val algorithm=k.getString("algorithm")
                val publicKey=k.getString("public_key")
                val state=k.getString("state")
                val keyId=k.getString("key_id")
                require(purpose in setOf("config","access","release"))
                require(algorithm=="ed25519")
                require(state in setOf("active","next","retired"))
                val raw=Base64.getUrlDecoder().decode(publicKey)
                require(raw.size==32)
                val expected=MessageDigest.getInstance("SHA-256")
                    .digest(raw).take(8).joinToString(""){"%02x".format(it)}
                require(expected==keyId) { "Trust key id mismatch" }
                add(TrustedKey(purpose,keyId,publicKey,state))
            }
        }
        require(keys.count{it.purpose=="config"&&it.state=="active"}==1)
        require(keys.count{it.purpose=="access"&&it.state=="active"}==1)
        require(keys.count{it.purpose=="release"&&it.state=="active"}<=1)
        require(keys.map{it.purpose+"\u0000"+it.keyId}.distinct().size==keys.size)
        return VerifiedTrustBundle(version,expiresAt,keys)
    }
}
