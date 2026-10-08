import Foundation
import NetworkExtension
import WireGuardKit

@MainActor
final class TunnelManager: ObservableObject {
    @Published private(set) var status: NEVPNStatus = .invalid
    @Published private(set) var isBusy = false
    @Published var errorMessage: String?
    @Published private(set) var personalKeyEnabled = false
    @Published private(set) var personalKeyFingerprint: String?
    @Published private(set) var account: IOSAccountStatus?
    @Published private(set) var plans: [IOSPlan] = []
    @Published private(set) var paymentBusy = false
    @Published private(set) var pairingBusy = false
    @Published private(set) var pairingCode: IOSPairingCode?
    @Published private(set) var devices:[IOSClientDevice]=[]
    @Published private(set) var devicesBusy=false
    @Published private(set) var referral: IOSReferralStatus?
    @Published private(set) var referralBusy = false
    @Published private(set) var referralMessage: String?
    @Published private(set) var updateRequired = false
    @Published private(set) var updateBlocked = false
    @Published private(set) var availableVersion:String?
    @Published private(set) var updateMessage:String?

    private var manager: NETunnelProviderManager?
    private let personalKeys = PersonalKeyStore()
    private lazy var repository = IOSVPNRepository(personalKeys:personalKeys)

    var isConnected: Bool { status == .connected || status == .reasserting }
    var canChangePersonalKey: Bool {
        status == .disconnected || status == .invalid
    }
    var hasUsableAccess: Bool {
        guard let account else { return false }
        let raw=account.graceUntil ?? account.expiresAt
        return ISO8601DateFormatter().date(from:raw).map{$0>Date()} ?? true
    }

    var statusTitle: String {
        switch status {
        case .connected, .reasserting: return "Соединение защищено"
        case .connecting: return "Подключаем защиту"
        case .disconnecting: return "Отключаем защиту"
        default: return "Защита выключена"
        }
    }

    var statusHint: String {
        switch status {
        case .connected, .reasserting: return "Интернет-трафик проходит через защищённое соединение."
        case .connecting: return "Подбираем доступный сервер."
        case .disconnecting: return "Завершаем текущее соединение."
        default: return "Нажмите «Подключить», чтобы включить VPN."
        }
    }

    init() {
        personalKeyEnabled = UserDefaults.standard.bool(forKey: "personal_key_enabled")
        refreshPersonalKeyInfo()
        NotificationCenter.default.addObserver(
            forName:.NEVPNStatusDidChange,object:nil,queue:.main
        ){[weak self] _ in
            Task{@MainActor in self?.status=self?.manager?.connection.status ?? .invalid}
        }
    }

    func reload() async {
        do {
            let managers=try await NETunnelProviderManager.loadAll()
            manager=managers.first{
                ($0.protocolConfiguration as? NETunnelProviderProtocol)?.providerBundleIdentifier=="ru.vpnx3.app.PacketTunnel"
            }
            status=manager?.connection.status ?? .disconnected
            if status == .disconnected || status == .invalid {
                await cleanupPersistedSession()
            }
            await refreshAccount()
            await refreshDevices()
            await refreshReferral()
            await refreshUpdatePolicy()
        } catch {
            errorMessage="Не удалось прочитать настройки VPN"
        }
    }

    func connect() async {
        guard !isBusy else{return}
        if updateRequired{
            errorMessage=updateMessage ?? "Требуется обновить приложение"
            return
        }
        isBusy=true;defer{isBusy=false};errorMessage=nil
        var prepared:PreparedIOSConnection?
        do{
            let next=try await repository.prepareConnection(personalKeyEnabled:personalKeyEnabled)
            prepared=next
            let current=manager ?? NETunnelProviderManager()
            let proto=NETunnelProviderProtocol()
            proto.providerBundleIdentifier="ru.vpnx3.app.PacketTunnel"
            proto.serverAddress=next.serverAddress
            proto.providerConfiguration=next.providerConfiguration
            current.protocolConfiguration=proto
            current.localizedDescription="VPNX3"
            current.isEnabled=true
            try await current.saveAsync()
            try await current.loadAsync()
            manager=current
            try current.connection.startVPNTunnel()
            status=.connecting
        }catch{
            if let prepared{await repository.release(prepared)}
            errorMessage="Не удалось подготовить VPN-соединение"
        }
    }

    func disconnect() async {
        manager?.connection.stopVPNTunnel()
        status=.disconnecting
    }

    private func cleanupPersistedSession() async {
        guard let proto=manager?.protocolConfiguration as? NETunnelProviderProtocol,
              let values=proto.providerConfiguration,
              let sessionID=values["sessionID"] as? String,
              let sessionAPI=values["sessionAPI"] as? String else{return}
        await repository.release(sessionID:sessionID,sessionAPI:sessionAPI)
        proto.providerConfiguration=nil
        manager?.protocolConfiguration=proto
        try? await manager?.saveAsync()
    }

    func refreshUpdatePolicy() async {
        do{
            let registration=try await IOSControlClient().ensureRegistered()
            let decision=try await IOSControlClient().releaseDecision(deviceID:registration.deviceID)
            updateRequired=decision.required
            updateBlocked=decision.blocked
            availableVersion=decision.availableVersion
            updateMessage=decision.message
        }catch{
            // Update policy is non-destructive while unavailable. Existing tunnel
            // and last known app version continue to work.
        }
    }

    func refreshAccount() async {
        do{
            let (_,current,available)=try await repository.account()
            account=current
            plans=available
        }catch{
            account=nil
            plans=(try? await IOSControlClient().plans()) ?? []
        }
    }

    func startPayment(planID:String,autoRenew:Bool) async->URL?{
        guard !paymentBusy else{return nil}
        paymentBusy=true
        defer{paymentBusy=false}
        do{
            let payment=try await repository.createPayment(planID:planID,autoRenew:autoRenew)
            return payment.confirmationURL
        }catch{
            errorMessage="Не удалось создать платёж"
            return nil
        }
    }

    func refreshDevices() async {
        guard !devicesBusy else{return}
        devicesBusy=true;defer{devicesBusy=false}
        do{devices=try await repository.devices()}
        catch{ /* device list is secondary to VPN availability */ }
    }

    func revokeDevice(_ id:String) async {
        guard !devicesBusy else{return}
        devicesBusy=true;defer{devicesBusy=false}
        do{
            try await repository.revokeDevice(id)
            devices=try await repository.devices()
            await refreshAccount()
        }catch{errorMessage="Не удалось отключить устройство"}
    }

    func createPairingCode() async {
        guard !pairingBusy else{return}
        pairingBusy=true;defer{pairingBusy=false}
        do{pairingCode=try await repository.createPairingCode()}
        catch{errorMessage="Нельзя добавить ещё одно устройство"}
    }

    func claimPairingCode(_ code:String) async {
        let normalized=code.trimmingCharacters(in:.whitespacesAndNewlines)
        guard !normalized.isEmpty,!pairingBusy else{return}
        pairingBusy=true;defer{pairingBusy=false}
        do{
            account=try await repository.claimPairingCode(normalized)
            pairingCode=nil
            await refreshAccount()
            await refreshDevices()
        }catch{errorMessage="Не удалось привязать устройство"}
    }

    func refreshReferral() async {
        do{referral=try await repository.referralStatus()}
        catch{ /* referral is non-critical for VPN availability */ }
    }

    func ensureReferralCode() async {
        guard !referralBusy else{return}
        referralBusy=true
        referralMessage=nil
        defer{referralBusy=false}
        do{
            _=try await repository.referralCode()
            referral=try await repository.referralStatus()
        }catch{
            errorMessage="Не удалось создать код приглашения"
        }
    }

    func claimReferralCode(_ code:String) async {
        let normalized=code.trimmingCharacters(in:.whitespacesAndNewlines).uppercased()
        guard !normalized.isEmpty,!referralBusy else{return}
        referralBusy=true
        referralMessage=nil
        defer{referralBusy=false}
        do{
            let result=try await repository.claimReferralCode(normalized)
            referral=try await repository.referralStatus()
            referralMessage=result.rewardApplied
                ? "Бонус +\(result.rewardDays) дней начислен"
                : "Бонус \(result.rewardDays) дней сохранён и будет применён к доступу"
            await refreshAccount()
        }catch{
            errorMessage="Не удалось применить код. Проверьте код и условия акции."
        }
    }

    func setAutoRenew(_ enabled:Bool) async {
        do{
            account=try await repository.setAutoRenew(enabled)
        }catch{
            errorMessage=enabled
                ? "Сначала оплатите тариф с включённым автопродлением"
                : "Не удалось отключить автопродление"
        }
    }

    func setPersonalKeyEnabled(_ enabled:Bool) async {
        guard canChangePersonalKey && !isBusy else{return}
        do{
            if enabled{_ = try personalKeys.ensure()}
            personalKeyEnabled=enabled
            UserDefaults.standard.set(enabled,forKey:"personal_key_enabled")
            refreshPersonalKeyInfo()
        }catch{errorMessage="Не удалось изменить личный ключ"}
    }

    func rotatePersonalKey() async {
        guard canChangePersonalKey && !isBusy else{return}
        do{
            let info=try personalKeys.rotate()
            personalKeyEnabled=true
            UserDefaults.standard.set(true,forKey:"personal_key_enabled")
            let value=info.publicKey.base64Key
            personalKeyFingerprint=String(value.prefix(10))+"…"+String(value.suffix(8))
        }catch{errorMessage="Не удалось заменить личный ключ"}
    }

    func deletePersonalKey() async {
        guard canChangePersonalKey && !isBusy else{return}
        do{
            try personalKeys.delete()
            personalKeyEnabled=false
            personalKeyFingerprint=nil
            UserDefaults.standard.set(false,forKey:"personal_key_enabled")
        }catch{errorMessage="Не удалось удалить личный ключ"}
    }

    private func refreshPersonalKeyInfo(){
        guard personalKeyEnabled else{personalKeyFingerprint=nil;return}
        guard let key=try? personalKeys.ensure() else{return}
        let value=key.publicKey.base64Key
        personalKeyFingerprint=String(value.prefix(10))+"…"+String(value.suffix(8))
    }
}

private extension NETunnelProviderManager {
    static func loadAll() async throws->[NETunnelProviderManager]{
        try await withCheckedThrowingContinuation{continuation in
            loadAllFromPreferences{managers,error in
                if let error{continuation.resume(throwing:error)}
                else{continuation.resume(returning:managers ?? [])}
            }
        }
    }
    func loadAsync() async throws{
        try await withCheckedThrowingContinuation{continuation in
            loadFromPreferences{error in
                if let error{continuation.resume(throwing:error)}else{continuation.resume()}
            }
        }
    }
    func saveAsync() async throws{
        try await withCheckedThrowingContinuation{continuation in
            saveToPreferences{error in
                if let error{continuation.resume(throwing:error)}else{continuation.resume()}
            }
        }
    }
}
