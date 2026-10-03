package ru.vpnx3.app.vpn

import android.content.Context
import com.wireguard.android.backend.GoBackend
import com.wireguard.android.backend.Tunnel
import com.wireguard.config.Config

class WireGuardController(context: Context) {
    private val backend = GoBackend(context.applicationContext)
    private val tunnel = AppTunnel()

    @Volatile
    private var state: Tunnel.State = Tunnel.State.DOWN

    fun connect(config: Config) {
        state = backend.setState(tunnel, Tunnel.State.UP, config)
        check(state == Tunnel.State.UP) { "WireGuard backend did not enter UP state" }
    }

    fun disconnect() {
        state = backend.setState(tunnel, Tunnel.State.DOWN, null)
    }

    fun isConnected(): Boolean = state == Tunnel.State.UP

    private inner class AppTunnel : Tunnel {
        override fun getName(): String = "vpnx3"

        override fun onStateChange(newState: Tunnel.State) {
            state = newState
        }
    }
}
