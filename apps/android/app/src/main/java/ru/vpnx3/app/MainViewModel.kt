package ru.vpnx3.app

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import ru.vpnx3.app.data.ClientAccountStatus
import ru.vpnx3.app.data.ClientPlan
import ru.vpnx3.app.data.ClientDevice
import ru.vpnx3.app.data.PreparedConnection
import ru.vpnx3.app.data.ReferralStatus
import ru.vpnx3.app.data.VpnRepository
import ru.vpnx3.app.security.PersonalKeyInfo

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
    val updateRequired: Boolean = false,
    val updateBlocked: Boolean = false,
    val updateMessage: String? = null,
    val plans: List<ClientPlan> = emptyList(),
    val paymentLoading: Boolean = false,
    val account: ClientAccountStatus? = null,
    val pairingCode: String? = null,
    val pairingCodeExpiresAt: String? = null,
    val pairingBusy: Boolean = false,
    val personalKeyEnabled: Boolean = false,
    val personalKeyInfo: PersonalKeyInfo? = null,
    val referral: ReferralStatus? = null,
    val referralBusy: Boolean = false,
    val referralMessage: String? = null,
    val devices: List<ClientDevice> = emptyList(),
    val devicesBusy: Boolean = false
)

private data class InitResult(
    val registration: ru.vpnx3.app.data.Registration,
    val update: ru.vpnx3.app.update.UpdateDecision,
    val plans: List<ClientPlan>,
    val connected: Boolean,
    val account: ClientAccountStatus?,
    val personalKeyEnabled: Boolean,
    val personalKeyInfo: PersonalKeyInfo?,
    val referral: ReferralStatus?
)

class MainViewModel(application: Application) : AndroidViewModel(application) {
    private val repository = VpnRepository(application)
    private val updates = UpdateRepository(application)
    private val mutableState = MutableStateFlow(MainUiState())
    val state: StateFlow<MainUiState> = mutableState.asStateFlow()

    @Volatile
    private var pending: PreparedConnection? = null

    init {
        initialize()
    }

    private fun initialize() {
        mutableState.value = mutableState.value.copy(
            connection = ConnectionState.PREPARING,
            error = null
        )
        viewModelScope.launch(Dispatchers.IO) {
            runCatching {
                val registration = repository.ensureRegistered()
                repository.latestConfig()
                val update = runCatching { repository.evaluateUpdate(registration.deviceId) }
                    .getOrDefault(ru.vpnx3.app.update.UpdateDecision())
                val plans = runCatching { repository.plans() }.getOrDefault(emptyList())
                val connected = repository.recoverConnectionState()
                val account=runCatching { repository.accountStatus() }.getOrNull()
                val personalEnabled=repository.personalKeyEnabled()
                val personalInfo=repository.personalKeyInfo()
                val referral=runCatching { repository.referralStatus() }.getOrNull()
                InitResult(registration,update,plans,connected,account,personalEnabled,personalInfo,referral)
            }.onSuccess { result ->
                val registration=result.registration
                mutableState.value = MainUiState(
                    registered = true,
                    trialExpiresAt = registration.trialExpiresAt,
                    connection = if(result.connected) ConnectionState.CONNECTED else ConnectionState.DISCONNECTED,
                    availableVersion = result.update.availableVersion,
                    updateRequired = result.update.required,
                    updateBlocked = result.update.blocked,
                    updateMessage = result.update.message,
                    plans = result.plans,
                    account = result.account,
                    personalKeyEnabled = result.personalKeyEnabled,
                    personalKeyInfo = result.personalKeyInfo,
                    referral = result.referral
                )
                refreshDevices()
            }.onFailure {
                mutableState.value = MainUiState(
                    connection = ConnectionState.ERROR,
                    error = "Не удалось подготовить VPNX3"
                )
            }
        }
    }

    fun retryInitialization() {
        if (mutableState.value.connection == ConnectionState.PREPARING) return
        initialize()
    }

    fun refreshAccount() {
        if(!mutableState.value.registered) return
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.accountStatus() }
                .onSuccess { mutableState.value=mutableState.value.copy(account=it,trialExpiresAt=null) }
            runCatching { repository.devices() }
                .onSuccess { mutableState.value=mutableState.value.copy(devices=it) }
        }
    }

    fun refreshDevices(){
        if(!mutableState.value.registered||mutableState.value.devicesBusy)return
        mutableState.value=mutableState.value.copy(devicesBusy=true)
        viewModelScope.launch(Dispatchers.IO){
            runCatching{repository.devices()}
                .onSuccess{mutableState.value=mutableState.value.copy(devices=it,devicesBusy=false)}
                .onFailure{mutableState.value=mutableState.value.copy(devicesBusy=false)}
        }
    }

    fun revokeDevice(deviceId:String){
        if(mutableState.value.devicesBusy)return
        mutableState.value=mutableState.value.copy(devicesBusy=true,error=null)
        viewModelScope.launch(Dispatchers.IO){
            runCatching{repository.revokeDevice(deviceId)}
                .onSuccess{
                    val devices=runCatching{repository.devices()}.getOrDefault(emptyList())
                    val account=runCatching{repository.accountStatus()}.getOrNull()
                    mutableState.value=mutableState.value.copy(
                        devices=devices,account=account,devicesBusy=false
                    )
                }
                .onFailure{
                    mutableState.value=mutableState.value.copy(
                        devicesBusy=false,error="Не удалось отключить устройство"
                    )
                }
        }
    }

    fun createPairingCode() {
        mutableState.value=mutableState.value.copy(pairingBusy=true,error=null)
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.createPairingCode() }
                .onSuccess {
                    mutableState.value=mutableState.value.copy(
                        pairingBusy=false,pairingCode=it.code,pairingCodeExpiresAt=it.expiresAt
                    )
                }
                .onFailure {
                    mutableState.value=mutableState.value.copy(
                        pairingBusy=false,error="Нельзя добавить ещё одно устройство"
                    )
                }
        }
    }

    fun claimPairingCode(code:String) {
        val normalized=code.trim()
        if(normalized.isEmpty()) return
        mutableState.value=mutableState.value.copy(pairingBusy=true,error=null)
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.claimPairingCode(normalized) }
                .onSuccess {
                    mutableState.value=mutableState.value.copy(
                        pairingBusy=false,account=it,trialExpiresAt=null,pairingCode=null,pairingCodeExpiresAt=null
                    )
                }
                .onFailure {
                    mutableState.value=mutableState.value.copy(
                        pairingBusy=false,error="Не удалось привязать устройство"
                    )
                }
        }
    }

    fun setPersonalKeyEnabled(enabled:Boolean) {
        if (mutableState.value.connection == ConnectionState.CONNECTED) {
            mutableState.value=mutableState.value.copy(error="Сначала отключите VPN")
            return
        }
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.setPersonalKeyEnabled(enabled) }
                .onSuccess { info ->
                    mutableState.value=mutableState.value.copy(
                        personalKeyEnabled=enabled,
                        personalKeyInfo=info,
                        error=null
                    )
                }
                .onFailure {
                    mutableState.value=mutableState.value.copy(error="Не удалось изменить личный ключ")
                }
        }
    }

    fun rotatePersonalKey() {
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.rotatePersonalKey() }
                .onSuccess { info ->
                    mutableState.value=mutableState.value.copy(
                        personalKeyEnabled=true,
                        personalKeyInfo=info,
                        error=null
                    )
                }
                .onFailure {
                    mutableState.value=mutableState.value.copy(error="Не удалось заменить личный ключ")
                }
        }
    }

    fun deletePersonalKey() {
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.deletePersonalKey() }
                .onSuccess {
                    mutableState.value=mutableState.value.copy(
                        personalKeyEnabled=false,
                        personalKeyInfo=null,
                        error=null
                    )
                }
                .onFailure {
                    mutableState.value=mutableState.value.copy(error="Не удалось удалить личный ключ")
                }
        }
    }

    fun refreshReferral() {
        if(!mutableState.value.registered) return
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.referralStatus() }
                .onSuccess { mutableState.value=mutableState.value.copy(referral=it) }
        }
    }

    fun ensureReferralCode() {
        if(mutableState.value.referralBusy) return
        mutableState.value=mutableState.value.copy(referralBusy=true,referralMessage=null,error=null)
        viewModelScope.launch(Dispatchers.IO) {
            runCatching {
                repository.referralCode()
                repository.referralStatus()
            }.onSuccess {
                mutableState.value=mutableState.value.copy(referralBusy=false,referral=it)
            }.onFailure {
                mutableState.value=mutableState.value.copy(
                    referralBusy=false,error="Не удалось создать код приглашения"
                )
            }
        }
    }

    fun claimReferralCode(code:String) {
        val normalized=code.trim().uppercase()
        if(normalized.isEmpty()||mutableState.value.referralBusy) return
        mutableState.value=mutableState.value.copy(referralBusy=true,referralMessage=null,error=null)
        viewModelScope.launch(Dispatchers.IO) {
            runCatching {
                val claim=repository.claimReferralCode(normalized)
                val status=repository.referralStatus()
                claim to status
            }.onSuccess { pair ->
                val claim=pair.first
                mutableState.value=mutableState.value.copy(
                    referralBusy=false,
                    referral=pair.second,
                    referralMessage=if(claim.rewardApplied)
                        "Бонус +${claim.rewardDays} дней начислен"
                    else
                        "Бонус ${claim.rewardDays} дней сохранён и будет применён к доступу"
                )
                refreshAccount()
            }.onFailure {
                mutableState.value=mutableState.value.copy(
                    referralBusy=false,
                    error="Не удалось применить код. Проверьте код и условия акции."
                )
            }
        }
    }

    fun setAutoRenew(enabled:Boolean) {
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.setAutoRenew(enabled) }
                .onSuccess { mutableState.value=mutableState.value.copy(account=it,error=null) }
                .onFailure {
                    mutableState.value=mutableState.value.copy(
                        error=if(enabled)
                            "Чтобы включить автопродление, сначала оплатите тариф с этой опцией"
                        else "Не удалось отключить автопродление"
                    )
                }
        }
    }

    fun startPayment(planId:String,autoRenew:Boolean,onReady:(String)->Unit) {
        mutableState.value=mutableState.value.copy(paymentLoading=true,error=null)
        viewModelScope.launch(Dispatchers.IO) {
            runCatching { repository.createPayment(planId,autoRenew) }
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
        if(mutableState.value.updateRequired){
            mutableState.value=mutableState.value.copy(
                connection=ConnectionState.ERROR,
                error=mutableState.value.updateMessage ?: "Требуется обновить приложение"
            )
            return
        }
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
