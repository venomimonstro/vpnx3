import Foundation
import NetworkExtension
import WireGuardKit
import Network

final class PacketTunnelProvider: NEPacketTunnelProvider {
    private lazy var adapter = WireGuardAdapter(with: self) { _, _ in }

    override func startTunnel(
        options: [String : NSObject]?,
        completionHandler: @escaping (Error?) -> Void
    ) {
        guard
            let proto = protocolConfiguration as? NETunnelProviderProtocol,
            let raw = proto.providerConfiguration?["wgQuickConfig"] as? String
        else {
            completionHandler(PacketTunnelError.invalidConfiguration)
            return
        }

        do {
            let config = try VPNX3WireGuardParser.parse(raw)
            adapter.start(tunnelConfiguration: config) { error in
                completionHandler(error)
            }
        } catch {
            completionHandler(error)
        }
    }

    override func stopTunnel(
        with reason: NEProviderStopReason,
        completionHandler: @escaping () -> Void
    ) {
        adapter.stop { _ in completionHandler() }
    }
}

enum PacketTunnelError: Error {
    case invalidConfiguration
}

enum VPNX3WireGuardParser {
    static func parse(_ text: String) throws -> TunnelConfiguration {
        var section = ""
        var interfaceValues:[String:String] = [:]
        var peerValues:[String:String] = [:]

        for rawLine in text.split(whereSeparator: \.isNewline) {
            let line = rawLine.trimmingCharacters(in: .whitespaces)
            if line.isEmpty || line.hasPrefix("#") { continue }
            if line == "[Interface]" { section = "interface"; continue }
            if line == "[Peer]" { section = "peer"; continue }
            guard let split = line.firstIndex(of: "=") else { continue }
            let key = line[..<split].trimmingCharacters(in: .whitespaces)
            let value = line[line.index(after: split)...].trimmingCharacters(in: .whitespaces)
            if section == "interface" { interfaceValues[key] = value }
            if section == "peer" { peerValues[key] = value }
        }

        guard let privateText=interfaceValues["PrivateKey"],
              let privateKey=PrivateKey(base64Key: privateText),
              let publicText=peerValues["PublicKey"],
              let publicKey=PublicKey(base64Key: publicText)
        else { throw PacketTunnelError.invalidConfiguration }

        var iface=InterfaceConfiguration(privateKey: privateKey)
        if let address=interfaceValues["Address"] {
            iface.addresses=address.split(separator:",").compactMap {
                IPAddressRange(from:String($0).trimmingCharacters(in:.whitespaces))
            }
        }
        if let mtu=interfaceValues["MTU"],let value=UInt16(mtu){iface.mtu=value}
        if let dns=interfaceValues["DNS"] {
            iface.dns=dns.split(separator:",").compactMap {
                DNSServer(from:String($0).trimmingCharacters(in:.whitespaces))
            }
        }

        var peer=PeerConfiguration(publicKey: publicKey)
        if let allowed=peerValues["AllowedIPs"] {
            peer.allowedIPs=allowed.split(separator:",").compactMap {
                IPAddressRange(from:String($0).trimmingCharacters(in:.whitespaces))
            }
        }
        if let endpoint=peerValues["Endpoint"] { peer.endpoint=Endpoint(from:endpoint) }
        if let keepalive=peerValues["PersistentKeepalive"],let value=UInt16(keepalive) {
            peer.persistentKeepAlive=value
        }
        return TunnelConfiguration(name:"VPNX3",interface:iface,peers:[peer])
    }
}
