import SwiftUI
import NetworkExtension

struct ContentView: View {
    @EnvironmentObject private var tunnel: TunnelManager
    @Environment(\.openURL) private var openURL
    @State private var showAccount=false
    @State private var autoRenewOnPurchase=false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 20) {
                    Text("VPNX3").font(.largeTitle.bold())
                    Text("Защищённое подключение без лишних настроек")
                        .foregroundStyle(.secondary)

                    VStack(spacing:12){
                        Text(tunnel.statusTitle).font(.title2.bold())
                        Text(tunnel.statusHint).foregroundStyle(.secondary)
                            .multilineTextAlignment(.center)

                        if let error=tunnel.errorMessage{
                            Text(error).foregroundStyle(.red)
                        }

                        Button {
                            Task {
                                if tunnel.isConnected { await tunnel.disconnect() }
                                else { await tunnel.connect() }
                            }
                        } label: {
                            Text(tunnel.isConnected ? "Отключить защиту" : "Подключить")
                                .frame(maxWidth:.infinity).padding(.vertical,8)
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(tunnel.isBusy)
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
                            autoRenewOnPurchase:$autoRenewOnPurchase,
                            onAutoRenew:{enabled in Task{await tunnel.setAutoRenew(enabled)}},
                            onBuy:{plan in
                                Task{
                                    if let url=await tunnel.startPayment(planID:plan.id,autoRenew:autoRenewOnPurchase){
                                        openURL(url)
                                    }
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
    }
}

private struct AccountView:View{
    let account:IOSAccountStatus?
    let plans:[IOSPlan]
    let paymentBusy:Bool
    @Binding var autoRenewOnPurchase:Bool
    let onAutoRenew:(Bool)->Void
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
            }else{
                Text("Активный тариф не найден").font(.headline)
            }

            Divider()
            Toggle("Автопродление после оплаты",isOn:$autoRenewOnPurchase)
            Text("По умолчанию выключено. При включении ЮKassa сохранит способ оплаты для следующих периодов.")
                .font(.footnote).foregroundStyle(.secondary)

            ForEach(plans){plan in
                Button {
                    onBuy(plan)
                } label: {
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

    private func displayDate(_ value:String)->String{
        String(value.prefix(10))
    }
}

private struct SettingsView: View {
    @EnvironmentObject private var tunnel: TunnelManager

    var body: some View {
        Form {
            Section("Личный ключ") {
                Toggle(
                    "Использовать ключ только этого устройства",
                    isOn: Binding(
                        get: { tunnel.personalKeyEnabled },
                        set: { enabled in Task { await tunnel.setPersonalKeyEnabled(enabled) } }
                    )
                )
                .disabled(tunnel.isConnected || tunnel.isBusy)

                if let fingerprint = tunnel.personalKeyFingerprint {
                    LabeledContent("Отпечаток", value: fingerprint)
                }

                Text("Приватный WireGuard-ключ хранится только в Keychain этого устройства и не отправляется в Control Plane.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .navigationTitle("Настройки")
    }
}
