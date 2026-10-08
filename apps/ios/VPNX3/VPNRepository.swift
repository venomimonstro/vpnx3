import Foundation
import WireGuardKit

struct PreparedIOSConnection {
    let serverAddress:String
    let sessionID:String
    let sessionAPI:String
    let keyMode:String
    let assignedIP:String
    let serverPublicKey:String
    let endpoint:String
    let mtu:Int
    let dns:[String]
    let keepalive:Int

    var providerConfiguration:[String:Any] {
        [
            "sessionID":sessionID,
            "sessionAPI":sessionAPI,
            "keyMode":keyMode,
            "assignedIP":assignedIP,
            "serverPublicKey":serverPublicKey,
            "endpoint":endpoint,
            "mtu":mtu,
            "dns":dns,
            "keepalive":keepalive
        ]
    }
}

final class IOSVPNRepository {
    private let personalKeys:PersonalKeyStore

    init(personalKeys:PersonalKeyStore){self.personalKeys=personalKeys}

    func account() async throws->(IOSRegistration,IOSAccountStatus,[IOSPlan]){
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        async let account=client.accountStatus(deviceID:registration.deviceID)
        async let plans=client.plans()
        return try await (registration,account,plans)
    }

    func devices() async throws->[IOSClientDevice]{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        return try await client.devices(deviceID:registration.deviceID)
    }

    func revokeDevice(_ targetID:String) async throws{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        try await client.revokeDevice(deviceID:registration.deviceID,targetID:targetID)
    }

    func createPairingCode() async throws->IOSPairingCode{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        return try await client.createPairingCode(deviceID:registration.deviceID)
    }

    func claimPairingCode(_ code:String) async throws->IOSAccountStatus{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        return try await client.claimPairingCode(deviceID:registration.deviceID,code:code)
    }

    func referralCode() async throws->String{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        return try await client.referralCode(deviceID:registration.deviceID)
    }

    func referralStatus() async throws->IOSReferralStatus{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        return try await client.referralStatus(deviceID:registration.deviceID)
    }

    func claimReferralCode(_ code:String) async throws->IOSReferralClaimResult{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        return try await client.claimReferralCode(deviceID:registration.deviceID,code:code)
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

    func prepareConnection(personalKeyEnabled:Bool) async throws->PreparedIOSConnection{
        let client=IOSControlClient()
        let registration=try await client.ensureRegistered()
        let config=try await client.latestVerifiedConfig()

        let publicKey:String
        let keyMode:String
        if personalKeyEnabled{
            publicKey=try personalKeys.ensure().publicKey.base64Key
            keyMode="personal"
        }else{
            publicKey=try StandardTunnelKeyStore().ensure().publicKey.base64Key
            keyMode="standard"
        }

        let lease=try await client.accessLease(
            deviceID:registration.deviceID,tunnelPublicKey:publicKey
        )
        return try await client.createWorkerSession(
            config:config,leaseEnvelope:lease,tunnelPublicKey:publicKey,keyMode:keyMode
        )
    }

    func release(_ prepared:PreparedIOSConnection) async {
        await release(sessionID:prepared.sessionID,sessionAPI:prepared.sessionAPI)
    }

    func release(sessionID:String,sessionAPI:String) async {
        let base=sessionAPI.hasSuffix("/") ? String(sessionAPI.dropLast()) : sessionAPI
        guard let url=URL(string:base+"/"+sessionID) else{return}
        var request=URLRequest(url:url)
        request.httpMethod="DELETE"
        request.timeoutInterval=5
        _=try? await URLSession.shared.data(for:request)
    }
}
