package ru.vpnx3.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import ru.vpnx3.app.data.ClientPlan
import ru.vpnx3.app.data.PreparedConnection
import ru.vpnx3.app.data.VpnRepository
import ru.vpnx3.app.update.UpdateRepository

enum class ConnectionState {
    PREPARING,
    DISCONNECTED,
    CONNECTING,
    CONNECTED,
    DISCONNECTING,
    ERROR
}

data class MainUiState(
    val registered: Boolean = false,
    val trialExpiresAt: String? = null,
    val connection: ConnectionState = ConnectionState.PREPARING,
    val error: String? = null,
    val availableVersion: String? = null,
    val plans: List<ClientPlan> = emptyList(),
    val paymentLoading: Boolean = false
)

private data class InitResult(
    val registration: ru.vpnx3.app.data.Registration,
    val update: ru.vpnx3.app.update.ReleaseInfo?,
    val plans: List<ClientPlan>,
    val connected: Boolean
)

class MainViewModel(application: Application) : AndroidViewModel(application) {
    private val repository = VpnRepository(application)
    private val updates = UpdateRepository()
    private val mutableState = MutableStateFlow(MainUiState())
    val state: StateFlow<MainUiState> = mutableState.asStateFlow()

    @Volatile
    private var pending: PreparedConnection? = null

    init {
        initialize()
    }

    private fun initialize() {
        viewModelScope.launch(Dispatchers.IO) {
            runCatching {
                val registration = repository.ensureRegistered()
                repository.latestConfig()
                val update = runCatching { updates.latest() }.getOrNull()
                val plans = runCatching { repository.plans() }.getOrDefault(emptyList())
                val connected = repository.recoverConnectionState()
                InitResult(registration,update,plans,connected)
            }.onSuccess { result ->
                val registration=result.registration
                mutableState.value = MainUiState(
                    registered = true,
                    trialExpiresAt = registration.trialExpiresAt,
                    connection = if(result.connected) ConnectionState.CONNECTED else ConnectionState.DISCONNECTED,
                    availableVersion = result.update?.version,
                    plans = result.plans
                )
            }.onFailure {
                mutableState.value = MainUiState(
                    connection = ConnectionState.ERROR,
                    error = "Не удалось подготовить VPNX3"
                )
            }
        }
    }

    fun startPayment(planId:String,onReady:(String)->Unit) {
        mutableState.value=mutableState.value.copy(paymentLoading=true,error=null)
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.createPayment(planId) }
                .onSuccess {
                    mutableState.value=mutableState.value.copy(paymentLoading=false)
                    onReady(it.confirmationUrl)
                }
                .onFailure {
                    mutableState.value=mutableState.value.copy(
                        paymentLoading=false,
                        error="Не удалось создать платёж"
                    )
                }
        }
    }

    fun prepareConnection(onPrepared: () -> Unit) {
        mutableState.value = mutableState.value.copy(
            connection = ConnectionState.CONNECTING,
            error = null
        )
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.prepareConnection() }
                .onSuccess {
                    pending = it
                    onPrepared()
                }
                .onFailure {
                    mutableState.value = mutableState.value.copy(
                        connection = ConnectionState.ERROR,
                        error = "Не удалось подготовить VPN-сессию"
                    )
                }
        }
    }

    fun cancelPrepared() {
        val prepared = pending ?: return
        pending = null
        viewModelScope.launch(Dispatchers.IO) {
            repository.release(prepared)
            mutableState.value = mutableState.value.copy(
                connection = ConnectionState.DISCONNECTED,
                error = null
            )
        }
    }

    fun connectPrepared() {
        val prepared = pending ?: run {
            mutableState.value = mutableState.value.copy(
                connection = ConnectionState.ERROR,
                error = "VPN-сессия не подготовлена"
            )
            return
        }
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.connect(prepared) }
                .onSuccess {
                    pending = null
                    mutableState.value = mutableState.value.copy(
                        connection = ConnectionState.CONNECTED,
                        error = null
                    )
                }
                .onFailure {
                    repository.disconnect()
                    pending = null
                    mutableState.value = mutableState.value.copy(
                        connection = ConnectionState.ERROR,
                        error = "Не удалось установить VPN-туннель"
                    )
                }
        }
    }

    fun disconnect() {
        mutableState.value = mutableState.value.copy(
            connection = ConnectionState.DISCONNECTING,
            error = null
        )
        viewModelScope.launch(Dispatchers.IO) {
            repository.disconnect()
            mutableState.value = mutableState.value.copy(
                connection = ConnectionState.DISCONNECTED
            )
        }
    }
}
