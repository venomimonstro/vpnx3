package ru.vpnx3.app.security

import com.google.crypto.tink.subtle.Ed25519Verify
import org.json.JSONObject
import java.security.MessageDigest
import java.time.Instant
import java.util.Base64

data class TrustKey(
    val purpose:String,
    val keyId:String,
    val publicKey:String,
    val state:String
)

data class VerifiedTrustBundle(
    val version:Long,
    val expiresAt:Instant,
    val keys:List<TrustKey>
) {
    fun verificationKeys(purpose:String):Map<String,String> =
        keys.filter {
            it.purpose==purpose && (it.state=="active" || it.state=="retired")
        }.associate { it.keyId to it.publicKey }
}

class TrustBundleVerifier(rootPublicKeyBase64Url:String) {
    private val root=Base64.getUrlDecoder().decode(rootPublicKeyBase64Url)
    private val verifier=Ed25519Verify(root)
    private val rootKeyId=MessageDigest.getInstance("SHA-256")
        .digest(root).take(8).joinToString(""){"%02x".format(it)}

    init { require(root.size==32) { "Trust root must contain 32 bytes" } }

    fun verify(rawEnvelope:String,minimumVersion:Long,now:Instant):VerifiedTrustBundle {
        val env=JSONObject(rawEnvelope)
        require(env.getString("key_id")==rootKeyId) { "Unexpected trust root" }
        val payload=Base64.getUrlDecoder().decode(env.getString("payload"))
        val signature=Base64.getUrlDecoder().decode(env.getString("signature"))
        verifier.verify(signature,payload)

        val json=JSONObject(payload.toString(Charsets.UTF_8))
        require(json.getInt("schema_version")==1) { "Unsupported trust bundle schema" }
        val version=json.getLong("version")
        require(version>0 && version>=minimumVersion) { "Trust bundle rollback detected" }
        val issued=Instant.parse(json.getString("issued_at"))
        val expires=Instant.parse(json.getString("expires_at"))
        require(!issued.isAfter(now.plusSeconds(300))) { "Trust bundle is from the future" }
        require(expires.isAfter(now)) { "Trust bundle expired" }
        require(expires.epochSecond-issued.epochSecond in 1..(366L*24*3600)) {
            "Invalid trust bundle validity"
        }

        val array=json.getJSONArray("keys")
        require(array.length() in 2..12) { "Invalid trust key count" }
        val seen=mutableSetOf<String>()
        val active=mutableMapOf<String,Int>()
        val keys=buildList {
            for(i in 0 until array.length()){
                val item=array.getJSONObject(i)
                val purpose=item.getString("purpose")
                val algorithm=item.getString("algorithm")
                val state=item.getString("state")
                val keyId=item.getString("key_id")
                val publicKey=item.getString("public_key")
                require(purpose in setOf("config","access","release"))
                require(algorithm=="ed25519")
                require(state in setOf("active","next","retired"))
                val raw=Base64.getUrlDecoder().decode(publicKey)
                require(raw.size==32)
                val expected=MessageDigest.getInstance("SHA-256")
                    .digest(raw).take(8).joinToString(""){"%02x".format(it)}
                require(expected==keyId) { "Trust key id mismatch" }
                require(seen.add("$purpose\u0000$keyId")) { "Duplicate trust key" }
                if(state=="active") active[purpose]=(active[purpose] ?: 0)+1
                add(TrustKey(purpose,keyId,publicKey,state))
            }
        }
        require(active["config"]==1 && active["access"]==1)
        require((active["release"] ?: 0)<=1)
        return VerifiedTrustBundle(version,expires,keys)
    }
}
