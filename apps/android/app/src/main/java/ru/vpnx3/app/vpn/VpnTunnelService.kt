package ru.vpnx3.app.vpn

import android.app.Service
import android.content.Intent
import android.net.VpnService
import android.os.IBinder

class VpnTunnelService : VpnService() {
    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // Транспорт подключается следующим шагом.
        // До этого сервис намеренно не показывает ложное состояние CONNECTED.
        stopSelf(startId)
        return Service.START_NOT_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = super.onBind(intent)
}
