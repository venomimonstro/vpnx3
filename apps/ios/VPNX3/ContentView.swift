import SwiftUI
import NetworkExtension

struct ContentView: View {
    @EnvironmentObject private var tunnel: TunnelManager

    var body: some View {
        NavigationStack {
            VStack(spacing: 24) {
                Spacer()
                Text("VPNX3")
                    .font(.largeTitle.bold())

                Text(tunnel.statusTitle)
                    .font(.title2.bold())

                Text(tunnel.statusHint)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(.secondary)

                if let error = tunnel.errorMessage {
                    Text(error)
                        .foregroundStyle(.red)
                        .multilineTextAlignment(.center)
                }

                Button {
                    Task {
                        if tunnel.isConnected {
                            await tunnel.disconnect()
                        } else {
                            await tunnel.connect()
                        }
                    }
                } label: {
                    Text(tunnel.isConnected ? "Отключить защиту" : "Подключить")
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 8)
                }
                .buttonStyle(.borderedProminent)
                .disabled(tunnel.isBusy)

                if tunnel.isBusy {
                    ProgressView()
                }

                Spacer()

                NavigationLink("Дополнительные настройки") {
                    SettingsView()
                }
            }
            .padding(24)
        }
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
                        set: { enabled in
                            Task { await tunnel.setPersonalKeyEnabled(enabled) }
                        }
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
