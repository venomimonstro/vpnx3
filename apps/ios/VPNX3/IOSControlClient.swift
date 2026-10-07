import Foundation
import WireGuardKit

struct IOSRegistration {
    let userID: String
    let deviceID: String
}

enum IOSControlError: Error {
    case runtimeNotConfigured
    case invalidResponse
    case invalidSignature
    case noWorker
    case unsupported
    case rollbackDetected
    case expiredConfiguration
}

final class IOSControlClient {
    private let runtime = IOSRuntimeConfig.current
    private let configStore=IOSConfigStore()

    func ensureRegistered() async throws -> IOSRegistration {
        guard runtime.controlURL.scheme=="https",!runtime.configPublicKey.isEmpty else {
            throw IOSControlError.runtimeNotConfigured
        }
        return try await IOSDeviceIdentity.shared.registration(controlURL: runtime.controlURL)
    }

    func latestVerifiedConfig() async throws -> IOSVerifiedConfig {
        let verifier=IOSConfigVerifier(publicKeyBase64:runtime.configPublicKey)
        let minimum=configStore.highestVersion
        do {
            guard let url=URL(string:"/api/v1/config/latest",relativeTo:runtime.controlURL)?.absoluteURL else {
                throw IOSControlError.runtimeNotConfigured
            }
            let (data,response)=try await URLSession.shared.data(from:url)
            guard (response as? HTTPURLResponse)?.statusCode==200 else { throw IOSControlError.invalidResponse }
            let verified=try verifier.verify(envelopeData:data,minimumVersion:minimum)
            configStore.envelope=data
            configStore.highestVersion=max(minimum,verified.version)
            return verified
        } catch {
            guard let cached=configStore.envelope else{throw error}
            return try verifier.verify(envelopeData:cached,minimumVersion:configStore.highestVersion)
        }
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
        guard let leaseObject=try JSONSerialization.jsonObject(with:Data(leaseEnvelope.utf8)) as? [String:Any] else {
            throw IOSControlError.invalidResponse
        }
        let routes=IOSRouting.candidates(config)
        guard !routes.isEmpty else{throw IOSControlError.noWorker}

        var lastError:Error=IOSControlError.noWorker
        for route in routes {
            do {
                guard let sessionURL=route.sessionAPI.httpsURL else{continue}
                var request=URLRequest(url:sessionURL)
                request.httpMethod="POST"
                request.setValue("application/json",forHTTPHeaderField:"Content-Type")
                request.setValue("application/json",forHTTPHeaderField:"Accept")
                request.httpBody=try JSONSerialization.data(withJSONObject:[
                    "lease":leaseObject,
                    "client_public_key":tunnelPrivateKey.publicKey.base64Key
                ])

                let (data,response)=try await URLSession.shared.data(for:request)
                guard let http=response as? HTTPURLResponse,(200..<300).contains(http.statusCode),
                      let object=try JSONSerialization.jsonObject(with:data) as? [String:Any],
                      let id=object["id"] as? String,
                      let sessionConfig=object["config"] as? [String:Any],
                      let assigned=sessionConfig["assigned_ip"] as? String,
                      let serverKey=sessionConfig["server_public_key"] as? String,
                      let endpoint=sessionConfig["endpoint"] as? String,
                      endpoint==route.wireGuard.hostPort
                else{throw IOSControlError.invalidResponse}

                let network=config.json["network"] as? [String:Any]
                let mtu=(network?["mtu"] as? NSNumber)?.intValue ?? 1280
                let keepalive=(network?["persistent_keepalive_seconds"] as? NSNumber)?.intValue ?? 25
                let dns=(network?["dns_servers"] as? [String]) ?? []
                let dnsLine=dns.isEmpty ? "" : "DNS = "+dns.joined(separator:", ")+"\n"
                let address=assigned.contains("/") ? assigned : assigned+"/32"

                let wg="""
                [Interface]
                PrivateKey = \(tunnelPrivateKey.base64Key)
                Address = \(address)
                MTU = \(min(max(mtu,576),1500))
                \(dnsLine)[Peer]
                PublicKey = \(serverKey)
                AllowedIPs = 0.0.0.0/0, ::/0
                Endpoint = \(endpoint)
                PersistentKeepalive = \(min(max(keepalive,0),120))
                """

                return PreparedIOSConnection(
                    wgQuickConfig:wg,
                    serverAddress:route.wireGuard.host,
                    sessionID:id,
                    sessionAPI:sessionURL.absoluteString
                )
            } catch {
                lastError=error
            }
        }
        throw lastError
    }
}
