import Foundation
import CryptoKit

struct IOSUpdateDecision {
    let availableVersion:String?
    let required:Bool
    let blocked:Bool
    let message:String?
}

struct IOSReleasePolicyVerifier {
    let publicKeyBase64:String

    func decision(
        envelopeData:Data,
        target:String,
        currentVersion:String,
        deviceID:String
    ) throws->IOSUpdateDecision {
        guard
            let envelope=try JSONSerialization.jsonObject(with:envelopeData) as? [String:Any],
            let payloadText=envelope["payload"] as? String,
            let signatureText=envelope["signature"] as? String,
            let keyID=envelope["key_id"] as? String,
            let payload=Data(vpnx3Base64URL:payloadText),
            let signature=Data(vpnx3Base64URL:signatureText),
            let keyData=Data(vpnx3Base64URL:publicKeyBase64),
            keyData.count==32
        else{throw IOSControlError.invalidResponse}

        let expected=SHA256.hash(data:keyData).prefix(8).map{String(format:"%02x",$0)}.joined()
        guard keyID==expected else{throw IOSControlError.invalidSignature}
        let key=try Curve25519.Signing.PublicKey(rawRepresentation:keyData)
        guard key.isValidSignature(signature,for:payload) else{throw IOSControlError.invalidSignature}
        guard let p=try JSONSerialization.jsonObject(with:payload) as? [String:Any],
              (p["schema_version"] as? NSNumber)?.intValue==1,
              p["target"] as? String==target,
              let issuedText=p["issued_at"] as? String,
              let expiresText=p["expires_at"] as? String,
              let rollout=(p["rollout_percent"] as? NSNumber)?.intValue,
              (0...100).contains(rollout)
        else{throw IOSControlError.invalidResponse}

        let formatter=ISO8601DateFormatter()
        let now=Date()
        guard let issued=formatter.date(from:issuedText),
              let expires=formatter.date(from:expiresText),
              issued<=now.addingTimeInterval(300),
              expires>now else{throw IOSControlError.expiredConfiguration}

        let minimum=(p["minimum_supported_version"] as? String) ?? ""
        let recommended=(p["recommended_version"] as? String) ?? ""
        let blocked=(p["blocked_versions"] as? [String]) ?? []
        let message=(p["message"] as? String)?.trimmingCharacters(in:.whitespacesAndNewlines)

        let isBlocked=blocked.contains(currentVersion)
        let belowMinimum=!minimum.isEmpty && compare(currentVersion,minimum)<0
        if isBlocked||belowMinimum{
            return IOSUpdateDecision(
                availableVersion:recommended.isEmpty ? nil:recommended,
                required:true,
                blocked:isBlocked,
                message:(message?.isEmpty==false) ? message :
                    (isBlocked ? "Эта версия приложения отключена. Установите обновление." :
                     "Для продолжения работы требуется обновить VPNX3.")
            )
        }

        guard !recommended.isEmpty,compare(currentVersion,recommended)<0 else{
            return IOSUpdateDecision(availableVersion:nil,required:false,blocked:false,message:nil)
        }
        let cohort=stableCohort(deviceID:deviceID,target:target,version:recommended)
        if cohort<rollout{
            return IOSUpdateDecision(
                availableVersion:recommended,required:false,blocked:false,
                message:(message?.isEmpty==false) ? message:nil
            )
        }
        return IOSUpdateDecision(availableVersion:nil,required:false,blocked:false,message:nil)
    }

    private func stableCohort(deviceID:String,target:String,version:String)->Int{
        let bytes=Data((deviceID+"\0"+target+"\0"+version).utf8)
        let digest=Array(SHA256.hash(data:bytes))
        return ((Int(digest[0])<<8)|Int(digest[1]))%100
    }

    private func compare(_ a:String,_ b:String)->Int{
        let aa=parse(a),bb=parse(b),count=max(aa.count,bb.count)
        for i in 0..<count{
            let av=i<aa.count ? aa[i]:0
            let bv=i<bb.count ? bb[i]:0
            if av != bv{return av<bv ? -1:1}
        }
        return 0
    }

    private func parse(_ value:String)->[Int]{
        value.trimmingCharacters(in:.whitespacesAndNewlines)
            .trimmingCharacters(in:CharacterSet(charactersIn:"v"))
            .split(separator:"-",maxSplits:1).first?
            .split(separator:".").map{Int($0) ?? 0} ?? [0]
    }
}

private extension Data {
    init?(vpnx3Base64URL value:String){
        var text=value.replacingOccurrences(of:"-",with:"+").replacingOccurrences(of:"_",with:"/")
        let remainder=text.count%4
        if remainder != 0{text += String(repeating:"=",count:4-remainder)}
        self.init(base64Encoded:text)
    }
}
