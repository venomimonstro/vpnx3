package ru.vpnx3.app

import android.content.Intent
import android.net.Uri
import android.net.VpnService
import android.os.Bundle
import android.provider.Settings
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
        if (result.resultCode == RESULT_OK) currentViewModel?.connectPrepared()
        else currentViewModel?.cancelPrepared()
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
                    onVpnSettings = { startActivity(Intent(Settings.ACTION_VPN_SETTINGS)) },
                    onBuy = { planId ->
                        vm.startPayment(planId) { url ->
                            runOnUiThread {
                                startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
                            }
                        }
                    }
                )
            }
        }
    }

    private fun requestVpnPermission(vm: MainViewModel) {
        val intent = VpnService.prepare(this)
        if (intent == null) vm.connectPrepared() else vpnPermission.launch(intent)
    }
}

@Composable
private fun HomeScreen(
    state: MainUiState,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    onCreatePairingCode: () -> Unit,
    onClaimPairingCode: (String) -> Unit,
    onVpnSettings: () -> Unit,
    onBuy: (String) -> Unit
) {
    val busy = state.connection == ConnectionState.PREPARING ||
        state.connection == ConnectionState.CONNECTING ||
        state.connection == ConnectionState.DISCONNECTING

    var showAccount by remember { mutableStateOf(false) }
    var showExtra by remember { mutableStateOf(false) }
    var showPairDialog by remember { mutableStateOf(false) }
    var pairInput by remember { mutableStateOf("") }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp, vertical = 32.dp),
        verticalArrangement = Arrangement.Top,
        horizontalAlignment = Alignment.CenterHorizontally
    ) {
        Text("VPNX3", style = MaterialTheme.typography.headlineLarge)
        Text(
            "Защищённое подключение без лишних настроек",
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )

        Spacer(Modifier.height(28.dp))

        Card(modifier = Modifier.fillMaxWidth()) {
            Column(
                modifier = Modifier.padding(20.dp),
                horizontalAlignment = Alignment.CenterHorizontally
            ) {
                Text(connectionTitle(state.connection), style = MaterialTheme.typography.headlineSmall)
                Spacer(Modifier.height(6.dp))
                Text(
                    connectionHint(state.connection),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )

                state.error?.let {
                    Spacer(Modifier.height(12.dp))
                    Text(it, color = MaterialTheme.colorScheme.error)
                }

                Spacer(Modifier.height(20.dp))

                if (busy || state.paymentLoading) {
                    CircularProgressIndicator()
                    Spacer(Modifier.height(8.dp))
                    Text(
                        if (state.paymentLoading) "Открываем оплату…" else "Подождите немного…",
                        style = MaterialTheme.typography.bodySmall
                    )
                } else if (state.connection == ConnectionState.CONNECTED) {
                    OutlinedButton(modifier = Modifier.fillMaxWidth(), onClick = onDisconnect) {
                        Text("Отключить защиту")
                    }
                } else {
                    Button(
                        modifier = Modifier.fillMaxWidth(),
                        enabled = state.registered,
                        onClick = onConnect
                    ) {
                        Text("Подключить")
                    }
                }
            }
        }

        Spacer(Modifier.height(16.dp))
        accessSummary(state)?.let {
            Text(
                it,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }

        state.availableVersion?.let {
            Spacer(Modifier.height(6.dp))
            Text("Доступно обновление: версия $it", style = MaterialTheme.typography.bodySmall)
        }

        Spacer(Modifier.height(20.dp))
        HorizontalDivider()
        Spacer(Modifier.height(8.dp))

        TextButton(modifier = Modifier.fillMaxWidth(), onClick = { showAccount = !showAccount }) {
            Text(if (showAccount) "Скрыть тариф и устройства" else "Тариф и устройства")
        }

        if (showAccount) {
            AccountSection(
                state = state,
                onCreatePairingCode = onCreatePairingCode,
                onOpenPairDialog = { showPairDialog = true },
                onBuy = onBuy
            )
        }

        TextButton(modifier = Modifier.fillMaxWidth(), onClick = { showExtra = !showExtra }) {
            Text(if (showExtra) "Скрыть дополнительные настройки" else "Дополнительные настройки")
        }

        if (showExtra) {
            Card(modifier = Modifier.fillMaxWidth()) {
                Column(Modifier.padding(16.dp)) {
                    Text("Постоянная защита", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(6.dp))
                    Text(
                        "Android умеет автоматически держать VPN включённым и блокировать интернет, если защищённое соединение оборвалось.",
                        style = MaterialTheme.typography.bodySmall
                    )
                    Spacer(Modifier.height(10.dp))
                    OutlinedButton(modifier = Modifier.fillMaxWidth(), onClick = onVpnSettings) {
                        Text("Открыть системные настройки VPN")
                    }
                }
            }
        }

        if (showPairDialog) {
            AlertDialog(
                onDismissRequest = { showPairDialog = false },
                title = { Text("Привязать устройство") },
                text = {
                    Column {
                        Text("Введите одноразовый код, показанный на другом устройстве вашего аккаунта.")
                        Spacer(Modifier.height(10.dp))
                        OutlinedTextField(
                            value = pairInput,
                            onValueChange = { pairInput = it },
                            singleLine = true,
                            label = { Text("Код подключения") }
                        )
                    }
                },
                confirmButton = {
                    TextButton(onClick = {
                        val code = pairInput.trim()
                        if (code.isNotEmpty()) {
                            showPairDialog = false
                            onClaimPairingCode(code)
                            pairInput = ""
                        }
                    }) { Text("Привязать") }
                },
                dismissButton = {
                    TextButton(onClick = { showPairDialog = false }) { Text("Отмена") }
                }
            )
        }
    }
}

@Composable
private fun AccountSection(
    state: MainUiState,
    onCreatePairingCode: () -> Unit,
    onOpenPairDialog: () -> Unit,
    onBuy: (String) -> Unit
) {
    Card(modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp)) {
            state.account?.let { account ->
                Text(account.planName ?: "Текущий доступ", style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(4.dp))
                Text("Доступ до ${displayDate(account.expiresAt)}")
                Text("Устройств: ${account.activeDevices} из ${account.deviceLimit}")
                if (account.autoRenew) {
                    Text(
                        "Автопродление включено",
                        color = MaterialTheme.colorScheme.primary,
                        style = MaterialTheme.typography.bodySmall
                    )
                }
                Spacer(Modifier.height(12.dp))

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    OutlinedButton(
                        modifier = Modifier.weight(1f),
                        enabled = !state.pairingBusy && account.activeDevices < account.deviceLimit,
                        onClick = onCreatePairingCode
                    ) { Text("Добавить") }
                    OutlinedButton(
                        modifier = Modifier.weight(1f),
                        enabled = !state.pairingBusy,
                        onClick = onOpenPairDialog
                    ) { Text("Привязать") }
                }

                state.pairingCode?.let {
                    Spacer(Modifier.height(10.dp))
                    Text("Код: $it", style = MaterialTheme.typography.titleMedium)
                    state.pairingCodeExpiresAt?.let { expires ->
                        Text(
                            "Одноразовый код действует до ${displayDateTime(expires)}",
                            style = MaterialTheme.typography.bodySmall
                        )
                    }
                }
            } ?: state.trialExpiresAt?.takeIf { it.isNotBlank() }?.let {
                Text("Пробный доступ", style = MaterialTheme.typography.titleMedium)
                Text("Действует до ${displayDate(it)}")
            } ?: Text("Активный доступ не найден")

            if (state.plans.isNotEmpty()) {
                Spacer(Modifier.height(16.dp))
                HorizontalDivider()
                Spacer(Modifier.height(12.dp))
                Text("Выбрать тариф", style = MaterialTheme.typography.titleMedium)
                state.plans.forEach { plan ->
                    Spacer(Modifier.height(8.dp))
                    val rubles = plan.priceMinor / 100.0
                    Button(
                        modifier = Modifier.fillMaxWidth(),
                        enabled = !state.paymentLoading,
                        onClick = { onBuy(plan.id) }
                    ) {
                        Text("${plan.name} — ${"%.0f".format(rubles)} ₽ на ${plan.billingPeriodDays} дней")
                    }
                }
            }
        }
    }
}

private fun connectionTitle(state: ConnectionState): String = when (state) {
    ConnectionState.PREPARING -> "Проверяем подключение"
    ConnectionState.DISCONNECTED -> "Защита выключена"
    ConnectionState.CONNECTING -> "Подключаем защиту"
    ConnectionState.CONNECTED -> "Соединение защищено"
    ConnectionState.DISCONNECTING -> "Отключаем защиту"
    ConnectionState.ERROR -> "Не удалось подключиться"
}

private fun connectionHint(state: ConnectionState): String = when (state) {
    ConnectionState.PREPARING -> "Готовим приложение к работе."
    ConnectionState.DISCONNECTED -> "Нажмите «Подключить», чтобы включить VPN."
    ConnectionState.CONNECTING -> "Подбираем доступный сервер."
    ConnectionState.CONNECTED -> "Интернет-трафик проходит через защищённое соединение."
    ConnectionState.DISCONNECTING -> "Завершаем текущее соединение."
    ConnectionState.ERROR -> "Проверьте интернет и попробуйте подключиться ещё раз."
}

private fun accessSummary(state: MainUiState): String? {
    state.account?.let { account ->
        val name = account.planName ?: "Доступ"
        return "$name · до ${displayDate(account.expiresAt)}"
    }
    return state.trialExpiresAt?.takeIf { it.isNotBlank() }?.let {
        "Пробный доступ · до ${displayDate(it)}"
    }
}

private fun displayDate(value: String): String =
    value.takeIf { it.length >= 10 }?.substring(0, 10) ?: value

private fun displayDateTime(value: String): String =
    value.replace("T", " ").take(16)
