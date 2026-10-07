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

    func account() async throws->(IOSRegistration,IOSAccountStatus,[IOSPlan]){
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        async let account=client.accountStatus(deviceID:registration.deviceID)
        async let plans=client.plans()
        return try await (registration,account,plans)
    }

    func createPayment(planID:String,autoRenew:Bool) async throws->IOSPaymentStart{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        return try await client.createPayment(deviceID:registration.deviceID,planID:planID,autoRenew:autoRenew)
    }

    func setAutoRenew(_ enabled:Bool) async throws->IOSAccountStatus{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        return try await client.setAutoRenew(deviceID:registration.deviceID,enabled:enabled)
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
