import SwiftUI
import NetworkExtension

struct ContentView: View {
    @EnvironmentObject private var tunnel: TunnelManager
    @Environment(\.openURL) private var openURL
    @Environment(\.scenePhase) private var scenePhase
    @State private var showAccount=false
    @State private var autoRenewOnPurchase=false
    @State private var showPairing=false
    @State private var pairingInput=""

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing:20) {
                    Text("VPNX3").font(.largeTitle.bold())
                    Text("Защищённое подключение без лишних настроек")
                        .foregroundStyle(.secondary)

                    if tunnel.updateRequired {
                        VStack(alignment:.leading,spacing:6){
                            Text(tunnel.updateBlocked ? "Эта версия отключена" : "Требуется обновление")
                                .font(.headline).foregroundStyle(.red)
                            Text(tunnel.updateMessage ?? "Установите актуальную версию VPNX3.")
                                .font(.footnote)
                            if let version=tunnel.availableVersion{
                                Text("Актуальная версия: \(version)").font(.footnote)
                            }
                        }
                        .padding()
                        .frame(maxWidth:.infinity,alignment:.leading)
                        .background(.thinMaterial,in:RoundedRectangle(cornerRadius:16))
                    } else if let version=tunnel.availableVersion {
                        Text("Доступно обновление: \(version)")
                            .font(.footnote).foregroundStyle(.secondary)
                    }

                    VStack(spacing:12){
                        Text(tunnel.statusTitle).font(.title2.bold())
                        Text(tunnel.statusHint)
                            .foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)

                        if let error=tunnel.errorMessage {
                            Text(error).foregroundStyle(.red)
                        }

                        Button {
                            Task {
                                if tunnel.isConnected {
                                    await tunnel.disconnect()
                                } else if tunnel.hasUsableAccess {
                                    await tunnel.connect()
                                } else {
                                    showAccount=true
                                }
                            }
                        } label: {
                            Text(
                                tunnel.isConnected
                                ? "Отключить защиту"
                                : (tunnel.updateRequired
                                    ? "Требуется обновление"
                                    : (tunnel.hasUsableAccess ? "Подключить" : "Выбрать тариф"))
                            )
                            .frame(maxWidth:.infinity)
                            .padding(.vertical,8)
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(tunnel.isBusy || (tunnel.updateRequired && !tunnel.isConnected))
                    }
                    .padding()
                    .background(.thinMaterial,in:RoundedRectangle(cornerRadius:16))

                    Button(showAccount ? "Скрыть тариф и устройства" : "Тариф и устройства"){
                        showAccount.toggle()
                    }

                    if showAccount {
                        AccountView(
                            account:tunnel.account,
                            plans:tunnel.plans,
                            paymentBusy:tunnel.paymentBusy,
                            pairingBusy:tunnel.pairingBusy,
                            pairingCode:tunnel.pairingCode,
                            devices:tunnel.devices,
                            devicesBusy:tunnel.devicesBusy,
                            autoRenewOnPurchase:$autoRenewOnPurchase,
                            onAutoRenew:{enabled in Task{await tunnel.setAutoRenew(enabled)}},
                            onCreatePairing:{Task{await tunnel.createPairingCode()}},
                            onClaimPairing:{showPairing=true},
                            onRevokeDevice:{id in Task{await tunnel.revokeDevice(id)}},
                            onBuy:{plan in
                                Task{
                                    if let url=await tunnel.startPayment(
                                        planID:plan.id,autoRenew:autoRenewOnPurchase
                                    ){openURL(url)}
                                }
                            }
                        )
                    }

                    NavigationLink("Дополнительные настройки") { SettingsView() }
                }
                .padding(24)
            }
            .refreshable { await tunnel.refreshAccount() }
        }
        .onChange(of:scenePhase){phase in
            if phase == .active {
                Task{
                    await tunnel.refreshAccount()
                    await tunnel.refreshUpdatePolicy()
                }
            }
        }
        .sheet(isPresented:$showPairing){
            NavigationStack {
                Form {
                    Section("Код подключения") {
                        TextField("Одноразовый код",text:$pairingInput)
                            .textInputAutocapitalization(.characters)
                            .autocorrectionDisabled()
                    }
                    Section {
                        Button("Привязать"){
                            let code=pairingInput
                            showPairing=false
                            pairingInput=""
                            Task{await tunnel.claimPairingCode(code)}
                        }
                        .disabled(pairingInput.trimmingCharacters(in:.whitespacesAndNewlines).isEmpty)
                    }
                }
                .navigationTitle("Привязать устройство")
                .toolbar {
                    ToolbarItem(placement:.cancellationAction){
                        Button("Отмена"){showPairing=false}
                    }
                }
            }
        }
    }
}

private struct AccountView:View{
    let account:IOSAccountStatus?
    let plans:[IOSPlan]
    let paymentBusy:Bool
    let pairingBusy:Bool
    let pairingCode:IOSPairingCode?
    let devices:[IOSClientDevice]
    let devicesBusy:Bool
    @Binding var autoRenewOnPurchase:Bool
    let onAutoRenew:(Bool)->Void
    let onCreatePairing:()->Void
    let onClaimPairing:()->Void
    let onRevokeDevice:(String)->Void
    let onBuy:(IOSPlan)->Void

    var body:some View{
        VStack(alignment:.leading,spacing:12){
            if let account{
                Text(account.planName ?? "Текущий доступ").font(.headline)
                Text("Доступ до \(displayDate(account.graceUntil ?? account.expiresAt))")
                Text("Устройств: \(account.activeDevices) из \(account.deviceLimit)")
                Toggle("Автопродление",isOn:Binding(
                    get:{account.autoRenew},
                    set:onAutoRenew
                ))

                HStack {
                    Button("Добавить устройство",action:onCreatePairing)
                        .disabled(pairingBusy || account.activeDevices>=account.deviceLimit)
                    Button("Ввести код",action:onClaimPairing)
                        .disabled(pairingBusy)
                }
                .buttonStyle(.bordered)

                if let pairingCode {
                    Text("Код: \(pairingCode.code)").font(.headline)
                    Text("Действует до \(displayDateTime(pairingCode.expiresAt))")
                        .font(.footnote).foregroundStyle(.secondary)
                }

                if !devices.isEmpty {
                    Divider()
                    Text("Ваши устройства").font(.headline)
                    ForEach(devices){device in
                        HStack {
                            VStack(alignment:.leading){
                                Text(device.displayName + (device.current ? " · это устройство" : ""))
                                Text(device.platform + (device.clientVersion.map{" · \($0)"} ?? ""))
                                    .font(.footnote).foregroundStyle(.secondary)
                            }
                            Spacer()
                            if !device.current && device.status=="active" {
                                Button("Отключить",role:.destructive){
                                    onRevokeDevice(device.id)
                                }
                                .disabled(devicesBusy)
                            }
                        }
                    }
                }
            }else{
                Text("Активный тариф не найден").font(.headline)
            }

            Divider()
            Toggle("Автопродление после оплаты",isOn:$autoRenewOnPurchase)
            Text("По умолчанию выключено. При включении ЮKassa сохранит способ оплаты для следующих периодов.")
                .font(.footnote).foregroundStyle(.secondary)

            ForEach(plans){plan in
                Button { onBuy(plan) } label: {
                    Text("\(plan.name) — \(plan.priceMinor/100) ₽ на \(plan.billingPeriodDays) дней")
                        .frame(maxWidth:.infinity)
                }
                .buttonStyle(.bordered)
                .disabled(paymentBusy)
            }
        }
        .padding()
        .background(.thinMaterial,in:RoundedRectangle(cornerRadius:16))
    }

    private func displayDate(_ value:String)->String { String(value.prefix(10)) }
    private func displayDateTime(_ value:String)->String {
        String(value.replacingOccurrences(of:"T",with:" ").prefix(16))
    }
}

private struct SettingsView: View {
    @EnvironmentObject private var tunnel: TunnelManager

    var body: some View {
        Form {
            Section("Личный ключ") {
                Toggle(
                    "Использовать ключ только этого устройства",
                    isOn:Binding(
                        get:{tunnel.personalKeyEnabled},
                        set:{enabled in Task{await tunnel.setPersonalKeyEnabled(enabled)}}
                    )
                )
                .disabled(!tunnel.canChangePersonalKey || tunnel.isBusy)

                if let fingerprint=tunnel.personalKeyFingerprint {
                    LabeledContent("Отпечаток",value:fingerprint)
                    Button("Заменить личный ключ"){
                        Task{await tunnel.rotatePersonalKey()}
                    }
                    .disabled(!tunnel.canChangePersonalKey)
                    Button("Удалить личный ключ",role:.destructive){
                        Task{await tunnel.deletePersonalKey()}
                    }
                    .disabled(!tunnel.canChangePersonalKey)
                }

                Text("Приватный WireGuard-ключ хранится только в Keychain этого устройства и не отправляется в Control Plane. После удаления восстановить его нельзя.")
                    .font(.footnote).foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Настройки")
    }
}
