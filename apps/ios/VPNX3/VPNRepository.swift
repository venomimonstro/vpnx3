import Foundation
import WireGuardKit

struct PreparedIOSConnection {
    let wgQuickConfig: String
    let serverAddress: String
    let sessionID: String
    let sessionAPI: String
}

final class IOSVPNRepository {
    private let personalKeys: PersonalKeyStore

    init(personalKeys: PersonalKeyStore) {
        self.personalKeys = personalKeys
    }

    func prepareConnection(personalKeyEnabled: Bool) async throws -> PreparedIOSConnection {
        // The complete Control API path is implemented in IOSControlClient.swift.
        // Keeping orchestration here makes the PacketTunnel extension independent
        // from user/account/business logic.
        let client = IOSControlClient()
        let registration = try await client.ensureRegistered()
        let config = try await client.latestVerifiedConfig()

        let key = personalKeyEnabled
            ? try personalKeys.ensure().privateKey
            : try StandardTunnelKeyStore().ensure()

        let lease = try await client.accessLease(
            deviceID: registration.deviceID,
            tunnelPublicKey: key.publicKey.base64Key
        )
        let prepared = try await client.createWorkerSession(
            config: config,
            leaseEnvelope: lease,
            tunnelPrivateKey: key
        )
        return prepared
    }
}
