package ru.vpnx3.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import ru.vpnx3.app.data.VpnRepository

data class MainUiState(
    val loading: Boolean = true,
    val registered: Boolean = false,
    val trialExpiresAt: String? = null,
    val leaseReady: Boolean = false,
    val error: String? = null
)

class MainViewModel(application: Application) : AndroidViewModel(application) {
    private val repository = VpnRepository(application)
    private val mutableState = MutableStateFlow(MainUiState())
    val state: StateFlow<MainUiState> = mutableState.asStateFlow()

    init {
        initialize()
    }

    private fun initialize() {
        viewModelScope.launch(Dispatchers.IO) {
            runCatching {
                val registration = repository.ensureRegistered()
                repository.latestConfig()
                registration
            }.onSuccess { registration ->
                mutableState.value = MainUiState(
                    loading = false,
                    registered = true,
                    trialExpiresAt = registration.trialExpiresAt
                )
            }.onFailure {
                mutableState.value = MainUiState(
                    loading = false,
                    error = "Не удалось подготовить соединение"
                )
            }
        }
    }

    fun prepareAccess(onReady: () -> Unit) {
        mutableState.value = mutableState.value.copy(loading = true, error = null)
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.obtainLease() }
                .onSuccess {
                    mutableState.value = mutableState.value.copy(
                        loading = false,
                        leaseReady = true
                    )
                    onReady()
                }
                .onFailure {
                    mutableState.value = mutableState.value.copy(
                        loading = false,
                        error = "Не удалось получить доступ"
                    )
                }
        }
    }
}
