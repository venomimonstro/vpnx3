package ru.vpnx3.app.data

import android.content.Context

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

    var requestSequence: Long
        get() = prefs.getLong("request_sequence", 0L)
        set(value) { prefs.edit().putLong("request_sequence", value).apply() }
}
