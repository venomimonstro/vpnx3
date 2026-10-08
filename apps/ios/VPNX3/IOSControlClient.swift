import Foundation
import WireGuardKit

struct IOSRegistration {
    let userID: String
    let deviceID: String
}

struct IOSPlan: Identifiable {
    let id:String
    let name:String
    let priceMinor:Int64
    let currency:String
    let billingPeriodDays:Int
    let deviceLimit:Int
}

struct IOSAccountStatus {
    let userID:String
    let entitlement:String
    let planName:String?
    let expiresAt:String
    let graceUntil:String?
    let deviceLimit:Int
    let activeDevices:Int
    let autoRenew:Bool
}

struct IOSPaymentStart {
    let paymentID:String
    let confirmationURL:URL
}

struct IOSPairingCode {
    let code:String
    let expiresAt:String
}

struct IOSClientDevice: Identifiable {
    let id:String
    let platform:String
    let displayName:String
    let status:String
    let clientVersion:String?
    let lastSeenAt:String?
    let current:Bool
}

struct IOSReferralStatus {
    let code:String?
    let claimed30d:Int64
    let qualified30d:Int64
    let rewardDaysGranted:Int64
    let referredBy:String?
}

struct IOSReferralClaimResult {
    let rewardDays:Int
    let rewardApplied:Bool
    let referrerRewardPending:Bool
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

    func releaseDecision(deviceID:String) async throws->IOSUpdateDecision{
        guard !runtime.releasePublicKey.isEmpty else{throw IOSControlError.runtimeNotConfigured}
        guard let url=URL(string:"/api/v1/releases/policy?target=ios_ipa",relativeTo:runtime.controlURL)?.absoluteURL else{
            throw IOSControlError.runtimeNotConfigured
        }
        let (data,response)=try await URLSession.shared.data(from:url)
        guard (response as? HTTPURLResponse)?.statusCode==200 else{throw IOSControlError.invalidResponse}
        let current=Bundle.main.object(forInfoDictionaryKey:"CFBundleShortVersionString") as? String ?? "0"
        return try IOSReleasePolicyVerifier(publicKeyBase64:runtime.releasePublicKey)
            .decision(envelopeData:data,target:"ios_ipa",currentVersion:current,deviceID:deviceID)
    }

    func plans() async throws->[IOSPlan]{
        guard let url=URL(string:"/api/v1/plans",relativeTo:runtime.controlURL)?.absoluteURL else{throw IOSControlError.runtimeNotConfigured}
        let (data,response)=try await URLSession.shared.data(from:url)
        guard (response as? HTTPURLResponse)?.statusCode==200,
              let root=try JSONSerialization.jsonObject(with:data) as? [String:Any],
              let rows=root["plans"] as? [[String:Any]] else{throw IOSControlError.invalidResponse}
        return rows.compactMap{p in
            guard let id=p["id"] as? String,
                  let name=p["name"] as? String,
                  let price=(p["price_minor"] as? NSNumber)?.int64Value,
                  let currency=p["currency"] as? String,
                  let period=(p["billing_period_days"] as? NSNumber)?.intValue,
                  let limit=(p["device_limit"] as? NSNumber)?.intValue else{return nil}
            return IOSPlan(id:id,name:name,priceMinor:price,currency:currency,billingPeriodDays:period,deviceLimit:limit)
        }
    }

    func accountStatus(deviceID:String) async throws->IOSAccountStatus{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/account/status",deviceID:deviceID,object:[:]
        )
        return try parseAccount(Data(raw.utf8))
    }

    func setAutoRenew(deviceID:String,enabled:Bool) async throws->IOSAccountStatus{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/account/auto-renew",deviceID:deviceID,
            object:["enabled":enabled]
        )
        return try parseAccount(Data(raw.utf8))
    }

    func devices(deviceID:String) async throws->[IOSClientDevice]{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/devices",deviceID:deviceID,object:[:]
        )
        guard let data=raw.data(using:.utf8),
              let root=try JSONSerialization.jsonObject(with:data) as? [String:Any],
              let rows=root["devices"] as? [[String:Any]] else{throw IOSControlError.invalidResponse}
        return rows.compactMap{d in
            guard let id=d["id"] as? String,
                  let platform=d["platform"] as? String,
                  let name=d["display_name"] as? String,
                  let status=d["status"] as? String else{return nil}
            return IOSClientDevice(
                id:id,platform:platform,displayName:name,status:status,
                clientVersion:d["client_version"] as? String,
                lastSeenAt:d["last_seen_at"] as? String,
                current:(d["current"] as? Bool) ?? false
            )
        }
    }

    func revokeDevice(deviceID:String,targetID:String) async throws{
        _ = try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/devices/revoke",deviceID:deviceID,
            object:["device_id":targetID]
        )
    }

    func createPairingCode(deviceID:String) async throws->IOSPairingCode{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/pairing-code",deviceID:deviceID,object:[:]
        )
        guard let data=raw.data(using:.utf8),
              let json=try JSONSerialization.jsonObject(with:data) as? [String:Any],
              let code=json["code"] as? String,
              let expires=json["expires_at"] as? String else{throw IOSControlError.invalidResponse}
        return IOSPairingCode(code:code,expiresAt:expires)
    }

    func claimPairingCode(deviceID:String,code:String) async throws->IOSAccountStatus{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/pairing-claim",deviceID:deviceID,
            object:["code":code]
        )
        return try parseAccount(Data(raw.utf8))
    }

    func referralCode(deviceID:String) async throws->String{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/referral/code",deviceID:deviceID,object:[:]
        )
        guard let data=raw.data(using:.utf8),
              let json=try JSONSerialization.jsonObject(with:data) as? [String:Any],
              let code=json["code"] as? String,!code.isEmpty else{throw IOSControlError.invalidResponse}
        return code
    }

    func referralStatus(deviceID:String) async throws->IOSReferralStatus{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/referral/status",deviceID:deviceID,object:[:]
        )
        guard let data=raw.data(using:.utf8),
              let json=try JSONSerialization.jsonObject(with:data) as? [String:Any]
        else{throw IOSControlError.invalidResponse}
        return IOSReferralStatus(
            code:(json["code"] as? String)?.nilIfEmpty,
            claimed30d:(json["claimed_30d"] as? NSNumber)?.int64Value ?? 0,
            qualified30d:(json["qualified_30d"] as? NSNumber)?.int64Value ?? 0,
            rewardDaysGranted:(json["reward_days_granted"] as? NSNumber)?.int64Value ?? 0,
            referredBy:(json["referred_by"] as? String)?.nilIfEmpty
        )
    }

    func claimReferralCode(deviceID:String,code:String) async throws->IOSReferralClaimResult{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/referral/claim",deviceID:deviceID,
            object:["code":code]
        )
        guard let data=raw.data(using:.utf8),
              let json=try JSONSerialization.jsonObject(with:data) as? [String:Any],
              let days=(json["reward_days"] as? NSNumber)?.intValue
        else{throw IOSControlError.invalidResponse}
        return IOSReferralClaimResult(
            rewardDays:days,
            rewardApplied:(json["reward_applied"] as? Bool) ?? false,
            referrerRewardPending:(json["referrer_reward_pending"] as? Bool) ?? true
        )
    }

    func createPayment(deviceID:String,planID:String,autoRenew:Bool) async throws->IOSPaymentStart{
        let raw=try await IOSDeviceIdentity.shared.signedJSON(
            controlURL:runtime.controlURL,path:"/api/v1/client/payments",deviceID:deviceID,
            object:["plan_id":planID,"auto_renew":autoRenew]
        )
        guard let data=raw.data(using:.utf8),
              let json=try JSONSerialization.jsonObject(with:data) as? [String:Any],
              let id=json["payment_id"] as? String,
              let urlText=json["confirmation_url"] as? String,
              let url=URL(string:urlText),url.scheme=="https" else{throw IOSControlError.invalidResponse}
        return IOSPaymentStart(paymentID:id,confirmationURL:url)
    }

    private func parseAccount(_ data:Data)throws->IOSAccountStatus{
        guard let j=try JSONSerialization.jsonObject(with:data) as? [String:Any],
              let user=j["user_id"] as? String,
              let entitlement=j["entitlement"] as? String,
              let expires=j["expires_at"] as? String,
              let limit=(j["device_limit"] as? NSNumber)?.intValue,
              let active=(j["active_devices"] as? NSNumber)?.intValue else{throw IOSControlError.invalidResponse}
        return IOSAccountStatus(
            userID:user,entitlement:entitlement,planName:j["plan_name"] as? String,
            expiresAt:expires,graceUntil:j["grace_until"] as? String,
            deviceLimit:limit,activeDevices:active,autoRenew:(j["auto_renew"] as? Bool) ?? false
        )
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
        config:IOSVerifiedConfig,
        leaseEnvelope:String,
        tunnelPublicKey:String,
        keyMode:String
    ) async throws->PreparedIOSConnection {
        guard keyMode=="standard"||keyMode=="personal",
              let leaseObject=try JSONSerialization.jsonObject(with:Data(leaseEnvelope.utf8)) as? [String:Any]
        else{throw IOSControlError.invalidResponse}

        let routes=IOSRouting.candidates(config)
        guard !routes.isEmpty else{throw IOSControlError.noWorker}
        var lastError:Error=IOSControlError.noWorker

        for route in routes{
            do{
                guard let sessionURL=route.sessionAPI.httpsURL else{continue}
                var request=URLRequest(url:sessionURL)
                request.httpMethod="POST"
                request.timeoutInterval=10
                request.setValue("application/json",forHTTPHeaderField:"Content-Type")
                request.setValue("application/json",forHTTPHeaderField:"Accept")
                request.httpBody=try JSONSerialization.data(withJSONObject:[
                    "lease":leaseObject,
                    "client_public_key":tunnelPublicKey
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
                return PreparedIOSConnection(
                    serverAddress:route.wireGuard.host,
                    sessionID:id,
                    sessionAPI:sessionURL.absoluteString,
                    keyMode:keyMode,
                    assignedIP:assigned.contains("/") ? assigned : assigned+"/32",
                    serverPublicKey:serverKey,
                    endpoint:endpoint,
                    mtu:min(max((network?["mtu"] as? NSNumber)?.intValue ?? 1280,576),1500),
                    dns:(network?["dns_servers"] as? [String]) ?? [],
                    keepalive:min(max((network?["persistent_keepalive_seconds"] as? NSNumber)?.intValue ?? 25,0),120)
                )
            }catch{lastError=error}
        }
        throw lastError
    }

}


private extension String {
    var nilIfEmpty:String? {
        isEmpty ? nil : self
    }
}
