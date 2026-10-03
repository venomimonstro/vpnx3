package ru.vpnx3.app.security

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.MessageDigest
import java.security.Signature
import java.security.spec.ECGenParameterSpec
import java.util.Base64

class DeviceIdentity {
    companion object {
        private const val KEYSTORE = "AndroidKeyStore"
        private const val ALIAS = "vpnx3-device-identity-v1"
        const val ALGORITHM = "ecdsa-p256-sha256"
    }

    private val keyStore: KeyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }

    fun ensureKey(): ByteArray {
        if (!keyStore.containsAlias(ALIAS)) {
            val generator = KeyPairGenerator.getInstance(
                KeyProperties.KEY_ALGORITHM_EC,
                KEYSTORE
            )
            val spec = KeyGenParameterSpec.Builder(
                ALIAS,
                KeyProperties.PURPOSE_SIGN or KeyProperties.PURPOSE_VERIFY
            )
                .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                .setDigests(KeyProperties.DIGEST_SHA256)
                .build()
            generator.initialize(spec)
            generator.generateKeyPair()
        }
        return keyStore.getCertificate(ALIAS).publicKey.encoded
    }

    fun publicKeyBase64Url(): String =
        Base64.getUrlEncoder().withoutPadding().encodeToString(ensureKey())

    fun sign(method: String, path: String, timestamp: String, body: ByteArray): String {
        ensureKey()
        val hash = MessageDigest.getInstance("SHA-256").digest(body)
        val hashHex = hash.joinToString("") { "%02x".format(it) }
        val canonical = listOf(method, path, timestamp, hashHex)
            .joinToString("\n")
            .toByteArray(Charsets.UTF_8)

        val privateKey = keyStore.getKey(ALIAS, null)
        val signer = Signature.getInstance("SHA256withECDSA")
        signer.initSign(privateKey as java.security.PrivateKey)
        signer.update(canonical)
        return Base64.getUrlEncoder().withoutPadding().encodeToString(signer.sign())
    }
}
