package ru.vpnx3.app.security

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import com.wireguard.crypto.Key
import com.wireguard.crypto.KeyPair
import java.security.KeyStore
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

class TunnelKeyStore(context: Context) {
    companion object {
        private const val KEYSTORE = "AndroidKeyStore"
        private const val WRAP_ALIAS = "vpnx3-wg-wrap-v1"
        private const val PREFS = "vpnx3_tunnel_secret"
        private const val CIPHERTEXT = "private_key_ciphertext"
        private const val IV = "private_key_iv"
    }

    private val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    private val keyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }

    fun getOrCreate(): KeyPair {
        val encrypted = prefs.getString(CIPHERTEXT, null)
        val iv = prefs.getString(IV, null)
        if (encrypted != null && iv != null) {
            val privateBase64 = decrypt(encrypted, iv)
            return KeyPair(Key.fromBase64(privateBase64))
        }

        val pair = KeyPair()
        persist(pair.privateKey.toBase64())
        return pair
    }

    private fun wrappingKey(): SecretKey {
        val existing = keyStore.getKey(WRAP_ALIAS, null)
        if (existing is SecretKey) return existing

        val generator = KeyGenerator.getInstance(
            KeyProperties.KEY_ALGORITHM_AES,
            KEYSTORE
        )
        generator.init(
            KeyGenParameterSpec.Builder(
                WRAP_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setRandomizedEncryptionRequired(true)
                .build()
        )
        return generator.generateKey()
    }

    private fun persist(privateBase64: String) {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, wrappingKey())
        val encrypted = cipher.doFinal(privateBase64.toByteArray(Charsets.UTF_8))
        prefs.edit()
            .putString(CIPHERTEXT, Base64.getEncoder().encodeToString(encrypted))
            .putString(IV, Base64.getEncoder().encodeToString(cipher.iv))
            .commit()
    }

    private fun decrypt(encryptedBase64: String, ivBase64: String): String {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        val iv = Base64.getDecoder().decode(ivBase64)
        cipher.init(
            Cipher.DECRYPT_MODE,
            wrappingKey(),
            GCMParameterSpec(128, iv)
        )
        val clear = cipher.doFinal(Base64.getDecoder().decode(encryptedBase64))
        return clear.toString(Charsets.UTF_8)
    }
}
