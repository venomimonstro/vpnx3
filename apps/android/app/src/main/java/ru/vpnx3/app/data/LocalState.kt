package ru.vpnx3.app.data

import android.content.Context
import org.json.JSONObject

data class ActiveSessionState(
    val sessionId: String,
    val scheme: String,
    val host: String,
    val port: Int,
    val path: String,
    val priority: Int
)

class LocalState(context: Context) {
    private val prefs = context.getSharedPreferences("vpnx3_state", Context.MODE_PRIVATE)

    var userId: String?
        get() = prefs.getString("user_id", null)
        set(value) { prefs.edit().putString("user_id", value).apply() }

    var deviceId: String?
        get() = prefs.getString("device_id", null)
        set(value) { prefs.edit().putString("device_id", value).apply() }

    var trialExpiresAt: String?
        get() = prefs.getString("trial_expires_at", null)
        set(value) { prefs.edit().putString("trial_expires_at", value).apply() }

    val requestSequence: Long
        get() = prefs.getLong("request_sequence", 0L)

    @Synchronized
    fun reserveNextRequestSequence(): Long {
        val next = requestSequence + 1L
        check(next > 0L) { "Request sequence overflow" }
        val persisted = prefs.edit()
            .putLong("request_sequence", next)
            .commit()
        check(persisted) { "Unable to persist request sequence" }
        return next
    }

    fun resetRequestSequence() {
        check(
            prefs.edit()
                .putLong("request_sequence", 0L)
                .commit()
        ) { "Unable to reset request sequence" }
    }

    var configEnvelope: String?
        get() = prefs.getString("config_envelope", null)
        set(value) { prefs.edit().putString("config_envelope", value).apply() }

    var highestConfigVersion: Long
        get() = prefs.getLong("highest_config_version", 0L)
        set(value) { prefs.edit().putLong("highest_config_version", value).apply() }

    var configMirrorUrls: List<String>
        get() = prefs.getString("config_mirror_urls", "")
            .orEmpty()
            .split('\n')
            .map { it.trim() }
            .filter { it.isNotEmpty() }
        set(value) {
            val safe=value.distinct().take(16).joinToString("\n")
            prefs.edit().putString("config_mirror_urls",safe).apply()
        }

    var personalKeyEnabled: Boolean
        get() = prefs.getBoolean("personal_key_enabled", false)
        set(value) {
            check(prefs.edit().putBoolean("personal_key_enabled", value).commit()) {
                "Unable to persist personal key mode"
            }
        }

    fun saveActiveSession(prepared: PreparedConnection) {
        val ep=prepared.workerRoute.sessionApi
        val raw=JSONObject()
            .put("session_id",prepared.sessionId)
            .put("scheme",ep.scheme)
            .put("host",ep.host)
            .put("port",ep.port)
            .put("path",ep.path)
            .put("priority",ep.priority)
            .toString()
        check(prefs.edit().putString("active_session",raw).commit()) {
            "Unable to persist active session"
        }
    }

    fun activeSession(): ActiveSessionState? {
        val raw=prefs.getString("active_session",null) ?: return null
        return runCatching {
            val json=JSONObject(raw)
            ActiveSessionState(
                sessionId=json.getString("session_id"),
                scheme=json.getString("scheme"),
                host=json.getString("host"),
                port=json.getInt("port"),
                path=json.optString("path",""),
                priority=json.optInt("priority",100)
            )
        }.getOrNull()
    }

    fun clearActiveSession() {
        prefs.edit().remove("active_session").commit()
    }
}
