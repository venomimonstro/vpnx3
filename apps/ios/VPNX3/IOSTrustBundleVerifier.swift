import Foundation
import CryptoKit

struct IOSTrustKey {
    let purpose:String
    let keyID:String
    let publicKey:String
    let state:String
}

struct IOSVerifiedTrustBundle {
    let version:Int64
    let expiresAt:Date
    let keys:[IOSTrustKey]

    func verificationKeys(_ purpose:String)->[String:String] {
        Dictionary(uniqueKeysWithValues:keys.compactMap { key in
            guard key.purpose==purpose && (key.state=="active" || key.state=="retired") else{return nil}
            return (key.keyID,key.publicKey)
        })
    }
}

struct IOSTrustBundleVerifier {
    let rootPublicKey:String

    func verify(_ envelopeData:Data,minimumVersion:Int64,now:Date=Date())throws->IOSVerifiedTrustBundle {
        guard
            let envelope=try JSONSerialization.jsonObject(with:envelopeData) as? [String:Any],
            let payloadText=envelope["payload"] as? String,
            let signatureText=envelope["signature"] as? String,
            let keyID=envelope["key_id"] as? String,
            let payload=Data(trustB64URL:payloadText),
            let signature=Data(trustB64URL:signatureText),
            let root=Data(trustB64URL:rootPublicKey),
            root.count==32
        else{throw IOSControlError.invalidResponse}

        let expected=SHA256.hash(data:root).prefix(8).map{String(format:"%02x",$0)}.joined()
        guard keyID==expected else{throw IOSControlError.invalidSignature}
        let verifier=try Curve25519.Signing.PublicKey(rawRepresentation:root)
        guard verifier.isValidSignature(signature,for:payload) else{throw IOSControlError.invalidSignature}

        guard let json=try JSONSerialization.jsonObject(with:payload) as? [String:Any],
              (json["schema_version"] as? NSNumber)?.intValue==1,
              let version=(json["version"] as? NSNumber)?.int64Value,
              version>0 && version>=minimumVersion,
              let issuedText=json["issued_at"] as? String,
              let expiresText=json["expires_at"] as? String,
              let rows=json["keys"] as? [[String:Any]],
              rows.count>=2 && rows.count<=12
        else{throw IOSControlError.rollbackDetected}

        let formatter=ISO8601DateFormatter()
        guard let issued=formatter.date(from:issuedText),let expires=formatter.date(from:expiresText),
              issued<=now.addingTimeInterval(300),expires>now,
              expires.timeIntervalSince(issued)>0,
              expires.timeIntervalSince(issued)<=366*24*3600
        else{throw IOSControlError.expiredConfiguration}

        var seen=Set<String>()
        var active:[String:Int]=[:]
        var keys:[IOSTrustKey]=[]
        for row in rows {
            guard let purpose=row["purpose"] as? String,
                  let algorithm=row["algorithm"] as? String,
                  let state=row["state"] as? String,
                  let kid=row["key_id"] as? String,
                  let pub=row["public_key"] as? String,
                  ["config","access","release"].contains(purpose),
                  algorithm=="ed25519",
                  ["active","next","retired"].contains(state),
                  let raw=Data(trustB64URL:pub),raw.count==32
            else{throw IOSControlError.invalidResponse}
            let calculated=SHA256.hash(data:raw).prefix(8).map{String(format:"%02x",$0)}.joined()
            guard calculated==kid,seen.insert(purpose+"\0"+kid).inserted else{throw IOSControlError.invalidResponse}
            if state=="active"{active[purpose,default:0]+=1}
            keys.append(IOSTrustKey(purpose:purpose,keyID:kid,publicKey:pub,state:state))
        }
        guard active["config"]==1,active["access"]==1,(active["release"] ?? 0)<=1 else{
            throw IOSControlError.invalidResponse
        }
        return IOSVerifiedTrustBundle(version:version,expiresAt:expires,keys:keys)
    }
}

private extension Data {
    init?(trustB64URL value:String){
        var text=value.replacingOccurrences(of:"-",with:"+").replacingOccurrences(of:"_",with:"/")
        if text.count%4 != 0{text += String(repeating:"=",count:4-text.count%4)}
        self.init(base64Encoded:text)
    }
}
