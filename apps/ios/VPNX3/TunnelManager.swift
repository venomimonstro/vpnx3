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

    private var manager: NETunnelProviderManager?
    private let personalKeys = PersonalKeyStore()

    var isConnected: Bool { status == .connected || status == .reasserting }

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
        } catch {
            errorMessage="Не удалось прочитать настройки VPN"
        }
    }

    func connect() async {
        guard !isBusy else{return}
        isBusy=true;defer{isBusy=false};errorMessage=nil
        do{
            let prepared=try await IOSVPNRepository(personalKeys:personalKeys)
                .prepareConnection(personalKeyEnabled:personalKeyEnabled)
            let current=manager ?? NETunnelProviderManager()
            let proto=NETunnelProviderProtocol()
            proto.providerBundleIdentifier="ru.vpnx3.app.PacketTunnel"
            proto.serverAddress=prepared.serverAddress
            proto.providerConfiguration=[
                "wgQuickConfig":prepared.wgQuickConfig,
                "sessionID":prepared.sessionID,
                "sessionAPI":prepared.sessionAPI
            ]
            current.protocolConfiguration=proto
            current.localizedDescription="VPNX3"
            current.isEnabled=true
            try await current.saveAsync()
            try await current.loadAsync()
            manager=current
            try current.connection.startVPNTunnel()
            status=.connecting
        }catch{
            errorMessage="Не удалось подготовить VPN-соединение"
        }
    }

    func disconnect() async {
        manager?.connection.stopVPNTunnel()
        status=.disconnecting
    }

    func setPersonalKeyEnabled(_ enabled:Bool) async {
        guard !isConnected && !isBusy else{return}
        do{
            if enabled{_ = try personalKeys.ensure()}
            personalKeyEnabled=enabled
            UserDefaults.standard.set(enabled,forKey:"personal_key_enabled")
            refreshPersonalKeyInfo()
        }catch{errorMessage="Не удалось изменить личный ключ"}
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
