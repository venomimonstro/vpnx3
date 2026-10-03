package ru.vpnx3.app.vpn

import android.content.Context
import com.wireguard.android.backend.GoBackend
import com.wireguard.android.backend.Tunnel
import com.wireguard.config.Config

class WireGuardController(context: Context) {
    private val backend = GoBackend(context.applicationContext)
    private val tunnel = AppTunnel()

    @Volatile
    private var lastObservedState: Tunnel.State = Tunnel.State.DOWN

    fun connect(config: Config) {
        val state = backend.setState(tunnel, Tunnel.State.UP, config)
        lastObservedState = state
        check(state == Tunnel.State.UP) { "WireGuard backend did not enter UP state" }
    }

    fun disconnect() {
        lastObservedState = backend.setState(tunnel, Tunnel.State.DOWN, null)
    }

    fun currentState(): Tunnel.State =
        runCatching { backend.getState(tunnel) }
            .onSuccess { lastObservedState = it }
            .getOrDefault(lastObservedState)

    fun isConnected(): Boolean = currentState() == Tunnel.State.UP

    private inner class AppTunnel : Tunnel {
        override fun getName(): String = "vpnx3"

        override fun onStateChange(newState: Tunnel.State) {
            lastObservedState = newState
        }
    }
}
