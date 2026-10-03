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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.TextButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
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

    override fun onResume() {
        super.onResume()
        currentViewModel?.refreshAccount()
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
                    onCreatePairingCode = vm::createPairingCode,
                    onClaimPairingCode = vm::claimPairingCode,
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
    onCreatePairingCode: () -> Unit,
    onClaimPairingCode: (String) -> Unit,
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

        state.account?.let { account ->
            Spacer(Modifier.height(8.dp))
            Text(
                if(account.planName!=null)
                    "${account.planName}: до ${account.expiresAt}"
                else
                    "Доступ ${account.entitlement}: до ${account.expiresAt}"
            )
            Text("Устройства: ${account.activeDevices} / ${account.deviceLimit}")
        } ?: state.trialExpiresAt?.takeIf { it.isNotBlank() }?.let {
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

        state.account?.let { account ->
            Spacer(Modifier.height(12.dp))
            Button(
                enabled=!state.pairingBusy && account.activeDevices < account.deviceLimit,
                onClick=onCreatePairingCode
            ) { Text("ДОБАВИТЬ УСТРОЙСТВО") }
            state.pairingCode?.let {
                Spacer(Modifier.height(8.dp))
                Text("Код подключения: $it")
                state.pairingCodeExpiresAt?.let { expires -> Text("Действует до $expires") }
            }
        }

        var showPairDialog by remember { mutableStateOf(false) }
        var pairInput by remember { mutableStateOf("") }
        Spacer(Modifier.height(8.dp))
        Button(enabled=!state.pairingBusy,onClick={showPairDialog=true}) {
            Text("ПРИВЯЗАТЬ ЭТО УСТРОЙСТВО")
        }
        if(showPairDialog) {
            AlertDialog(
                onDismissRequest={showPairDialog=false},
                title={Text("Код другого устройства")},
                text={
                    OutlinedTextField(
                        value=pairInput,
                        onValueChange={pairInput=it},
                        singleLine=true,
                        label={Text("Код подключения")}
                    )
                },
                confirmButton={
                    TextButton(onClick={
                        showPairDialog=false
                        onClaimPairingCode(pairInput)
                        pairInput=""
                    }) { Text("Привязать") }
                },
                dismissButton={
                    TextButton(onClick={showPairDialog=false}) { Text("Отмена") }
                }
            )
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
