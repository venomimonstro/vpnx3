import SwiftUI

@main
struct VPNX3App: App {
    @StateObject private var tunnel = TunnelManager()

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(tunnel)
                .task { await tunnel.reload() }
        }
    }
}
