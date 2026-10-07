package ru.vpnx3.app.security

import android.content.Context
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyInfo
import android.security.keystore.KeyProperties
import android.security.keystore.StrongBoxUnavailableException
import com.wireguard.crypto.Key
import com.wireguard.crypto.KeyPair
import java.security.KeyFactory
import java.security.KeyStore
import java.security.SecureRandom
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.SecretKeyFactory
import javax.crypto.spec.GCMParameterSpec

enum class PersonalKeyProtection {
    STRONGBOX,
    HARDWARE_BACKED,
    SOFTWARE_BACKED
}

data class PersonalKeyInfo(
    val publicKey: String,
    val protection: PersonalKeyProtection,
    val createdAtEpochMillis: Long
)

/**
 * Personal WireGuard identity.
 *
 * The WireGuard private key is generated only on this Android device and is
 * encrypted at rest by a non-exportable AES key in AndroidKeyStore. The AES
 * key is requested from StrongBox when supported. No method returns the
 * WireGuard private key as a String/ByteArray to callers.
 *
 * This profile is deliberately separated from normal app backup/sync flows.
 */
class PersonalTunnelKeyStore(context: Context) {
    companion object {
        private const val KEYSTORE = "AndroidKeyStore"
        private const val WRAP_ALIAS = "vpnx3-personal-wg-wrap-v1"
        private const val PREFS = "vpnx3_personal_tunnel_secret"
        private const val CIPHERTEXT = "private_key_ciphertext"
        private const val IV = "private_key_iv"
        private const val PUBLIC_KEY = "public_key"
        private const val CREATED_AT = "created_at"
    }

    private val appContext = context.applicationContext
    private val prefs = appContext.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    private val keyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }
    private val random = SecureRandom()

    fun ensure(): PersonalKeyInfo {
        val public = prefs.getString(PUBLIC_KEY, null)
        val createdAt = prefs.getLong(CREATED_AT, 0L)
        if (public != null && createdAt > 0L && keyStore.containsAlias(WRAP_ALIAS)) {
            // Verify ciphertext can still be opened. This detects restored prefs
            // without the device-bound AndroidKeyStore key.
            withPrivateKey { }
            return PersonalKeyInfo(public, protectionLevel(), createdAt)
        }

        delete()

        val pair = KeyPair()
        persist(pair.privateKey)
        val now = System.currentTimeMillis()
        prefs.edit()
            .putString(PUBLIC_KEY, pair.publicKey.toBase64())
            .putLong(CREATED_AT, now)
            .commit()

        return PersonalKeyInfo(pair.publicKey.toBase64(), protectionLevel(), now)
    }

    fun <T> withPrivateKey(block: (Key) -> T): T {
        val encrypted = prefs.getString(CIPHERTEXT, null)
            ?: error("Personal key is not initialized")
        val iv = prefs.getString(IV, null)
            ?: error("Personal key IV is missing")

        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(
            Cipher.DECRYPT_MODE,
            wrappingKey(),
            GCMParameterSpec(128, Base64.getDecoder().decode(iv))
        )
        val clear = cipher.doFinal(Base64.getDecoder().decode(encrypted))
        try {
            val key = Key.fromBase64(clear.toString(Charsets.US_ASCII))
            return block(key)
        } finally {
            clear.fill(0)
        }
    }

    fun rotate(): PersonalKeyInfo {
        delete()
        return ensure()
    }

    fun delete() {
        prefs.edit().clear().commit()
        if (keyStore.containsAlias(WRAP_ALIAS)) {
            keyStore.deleteEntry(WRAP_ALIAS)
        }
    }

    private fun persist(privateKey: Key) {
        val clear = privateKey.toBase64().toByteArray(Charsets.US_ASCII)
        try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, wrappingKey())
            val encrypted = cipher.doFinal(clear)
            prefs.edit()
                .putString(CIPHERTEXT, Base64.getEncoder().encodeToString(encrypted))
                .putString(IV, Base64.getEncoder().encodeToString(cipher.iv))
                .commit()
        } finally {
            clear.fill(0)
        }
    }

    private fun wrappingKey(): SecretKey {
        val existing = keyStore.getKey(WRAP_ALIAS, null)
        if (existing is SecretKey) return existing

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            try {
                return generateWrappingKey(strongBox = true)
            } catch (_: StrongBoxUnavailableException) {
                // Fall through to regular AndroidKeyStore.
            } catch (_: java.security.ProviderException) {
                // Vendor keystores may advertise StrongBox but fail allocation.
            }
        }
        return generateWrappingKey(strongBox = false)
    }

    private fun generateWrappingKey(strongBox: Boolean): SecretKey {
        val generator = KeyGenerator.getInstance(
            KeyProperties.KEY_ALGORITHM_AES,
            KEYSTORE
        )
        val builder = KeyGenParameterSpec.Builder(
            WRAP_ALIAS,
            KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
        )
            .setKeySize(256)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setRandomizedEncryptionRequired(true)

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P && strongBox) {
            builder.setIsStrongBoxBacked(true)
        }

        generator.init(builder.build())
        return generator.generateKey()
    }

    private fun protectionLevel(): PersonalKeyProtection {
        val key = wrappingKey()
        val factory = SecretKeyFactory.getInstance(key.algorithm, KEYSTORE)
        val info = factory.getKeySpec(key, KeyInfo::class.java) as KeyInfo
        return when {
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.S &&
                info.securityLevel == KeyProperties.SECURITY_LEVEL_STRONGBOX ->
                PersonalKeyProtection.STRONGBOX
            info.isInsideSecureHardware -> PersonalKeyProtection.HARDWARE_BACKED
            else -> PersonalKeyProtection.SOFTWARE_BACKED
        }
    }
}
