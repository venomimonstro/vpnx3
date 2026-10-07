import Foundation
import CryptoKit
import WireGuardKit

struct IOSRegistration {
    let userID: String
    let deviceID: String
}

struct IOSVerifiedConfig {
    let rawPayload: Data
    let json: [String: Any]
}

enum IOSControlError: Error {
    case runtimeNotConfigured
    case invalidResponse
    case invalidSignature
    case noWorker
    case unsupported
}

final class IOSControlClient {
    private let runtime = IOSRuntimeConfig.current

    func ensureRegistered() async throws -> IOSRegistration {
        guard !runtime.controlURL.isEmpty else { throw IOSControlError.runtimeNotConfigured }
        // Registration/signing parity with Android is implemented in the next
        // client-hardening step; this type intentionally fails closed until
        // the device identity exists instead of creating an unauthenticated path.
        return try await IOSDeviceIdentity.shared.registration(controlURL: runtime.controlURL)
    }

    func latestVerifiedConfig() async throws -> IOSVerifiedConfig {
        let url = runtime.controlURL.appendingPathComponent("/api/v1/config/latest")
        let (data,response) = try await URLSession.shared.data(from: url)
        guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw IOSControlError.invalidResponse }
        return try IOSConfigVerifier(publicKeyBase64: runtime.configPublicKey).verify(envelopeData: data)
    }

    func accessLease(deviceID: String, tunnelPublicKey: String) async throws -> String {
        try await IOSDeviceIdentity.shared.signedJSON(
            controlURL: runtime.controlURL,
            path: "/api/v1/client/lease",
            deviceID: deviceID,
            object: ["tunnel_public_key": tunnelPublicKey]
        )
    }

    func createWorkerSession(
        config: IOSVerifiedConfig,
        leaseEnvelope: String,
        tunnelPrivateKey: PrivateKey
    ) async throws -> PreparedIOSConnection {
        guard
            let workers = config.json["workers"] as? [[String: Any]],
            let worker = workers.first,
            let endpoints = worker["endpoints"] as? [[String: Any]],
            let session = endpoints.first(where: { ($0["kind"] as? String) == "session_api" }),
            let wireguard = endpoints.first(where: { ($0["kind"] as? String) == "wireguard" }),
            let sessionHost = session["host"] as? String,
            let sessionPort = session["port"] as? Int,
            let sessionPath = session["path"] as? String,
            let wgHost = wireguard["host"] as? String,
            let wgPort = wireguard["port"] as? Int
        else { throw IOSControlError.noWorker }

        let sessionURL = URL(string: "https://\(sessionHost):\(sessionPort)\(sessionPath)")!
        var request = URLRequest(url: sessionURL)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONSerialization.data(withJSONObject: [
            "lease": leaseEnvelope,
            "client_public_key": tunnelPrivateKey.publicKey.base64Key
        ])
        let (data,response) = try await URLSession.shared.data(for: request)
        guard (response as? HTTPURLResponse)?.statusCode == 201,
              let object = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              let id = object["session_id"] as? String,
              let assigned = object["assigned_ip"] as? String,
              let serverKey = object["server_public_key"] as? String
        else { throw IOSControlError.invalidResponse }

        let network = config.json["network"] as? [String: Any]
        let mtu = (network?["mtu"] as? Int) ?? 1280
        let keepalive = (network?["persistent_keepalive_seconds"] as? Int) ?? 25
        let dns = (network?["dns_servers"] as? [String]) ?? []
        let dnsLine = dns.isEmpty ? "" : "DNS = " + dns.joined(separator: ", ") + "\n"
        let address = assigned.contains("/") ? assigned : assigned + "/32"

        let wg = """
        [Interface]
        PrivateKey = \(tunnelPrivateKey.base64Key)
        Address = \(address)
        MTU = \(mtu)
        \(dnsLine)[Peer]
        PublicKey = \(serverKey)
        AllowedIPs = 0.0.0.0/0, ::/0
        Endpoint = \(wgHost):\(wgPort)
        PersistentKeepalive = \(keepalive)
        """

        return PreparedIOSConnection(
            wgQuickConfig: wg,
            serverAddress: wgHost,
            sessionID: id,
            sessionAPI: sessionURL.absoluteString
        )
    }
}
