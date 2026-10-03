package ru.vpnx3.app

import android.content.Intent
import android.net.Uri
import android.net.VpnService
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.viewmodel.compose.viewModel

class MainActivity : ComponentActivity() {
    private var currentViewModel: MainViewModel? = null

    private val vpnPermission = registerForActivityResult(
        ActivityResultContracts.StartActivityForResult()
    ) { result ->
        if (result.resultCode == RESULT_OK) {
            currentViewModel?.connectPrepared()
        } else {
            currentViewModel?.cancelPrepared()
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                val vm: MainViewModel = viewModel()
                currentViewModel = vm
                val state by vm.state.collectAsState()

                HomeScreen(
                    state = state,
                    onConnect = {
                        vm.prepareConnection {
                            runOnUiThread { requestVpnPermission(vm) }
                        }
                    },
                    onDisconnect = vm::disconnect,
                    onBuy = { planId ->
                        vm.startPayment(planId) { url ->
                            runOnUiThread {
                                startActivity(Intent(Intent.ACTION_VIEW,Uri.parse(url)))
                            }
                        }
                    }
                )
            }
        }
    }

    private fun requestVpnPermission(vm: MainViewModel) {
        val intent = VpnService.prepare(this)
        if (intent == null) {
            vm.connectPrepared()
        } else {
            vpnPermission.launch(intent)
        }
    }
}

@Composable
private fun HomeScreen(
    state: MainUiState,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onBuy: (String) -> Unit
) {
    val busy = state.connection == ConnectionState.PREPARING ||
        state.connection == ConnectionState.CONNECTING ||
        state.connection == ConnectionState.DISCONNECTING

    Column(
        modifier = Modifier.fillMaxSize().padding(32.dp),
        verticalArrangement = Arrangement.Center,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text("VPNX3", style = MaterialTheme.typography.headlineLarge)
        Spacer(Modifier.height(24.dp))

        Text(
            when (state.connection) {
                ConnectionState.PREPARING -> "Подготовка"
                ConnectionState.DISCONNECTED -> "Отключено"
                ConnectionState.CONNECTING -> "Подключение"
                ConnectionState.CONNECTED -> "Защищено"
                ConnectionState.DISCONNECTING -> "Отключение"
                ConnectionState.ERROR -> "Ошибка"
            }
        )

        state.trialExpiresAt?.takeIf { it.isNotBlank() }?.let {
            Spacer(Modifier.height(8.dp))
            Text("Пробный доступ до $it")
        }

        state.availableVersion?.let {
            Spacer(Modifier.height(8.dp))
            Text("Доступна версия $it")
        }

        state.error?.let {
            Spacer(Modifier.height(12.dp))
            Text(it, color = MaterialTheme.colorScheme.error)
        }

        if (state.plans.isNotEmpty()) {
            Spacer(Modifier.height(20.dp))
            Text("Тарифы", style = MaterialTheme.typography.titleMedium)
            state.plans.forEach { plan ->
                Spacer(Modifier.height(8.dp))
                val rubles = plan.priceMinor / 100.0
                Button(
                    enabled = !state.paymentLoading,
                    onClick = { onBuy(plan.id) }
                ) {
                    Text("${plan.name} — ${"%.2f".format(rubles)} ${plan.currency} / ${plan.billingPeriodDays} дн.")
                }
            }
        }

        Spacer(Modifier.height(24.dp))
        if (busy || state.paymentLoading) {
            CircularProgressIndicator()
        } else if (state.connection == ConnectionState.CONNECTED) {
            Button(onClick = onDisconnect) {
                Text("ОТКЛЮЧИТЬ")
            }
        } else {
            Button(
                enabled = state.registered,
                onClick = onConnect
            ) {
                Text("ПОДКЛЮЧИТЬ")
            }
        }
    }
}
