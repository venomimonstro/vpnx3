import Foundation
import CryptoKit

struct IOSVerifiedConfig {
    let version: Int64
    let expiresAt: Date
    let rawPayload: Data
    let json: [String: Any]
}

struct IOSConfigVerifier {
    let publicKeyBase64: String

    func verify(envelopeData:Data,minimumVersion:Int64,now:Date=Date())throws->IOSVerifiedConfig {
        guard !publicKeyBase64.isEmpty else{throw IOSControlError.runtimeNotConfigured}
        return try Self.verify(
            envelopeData:envelopeData,minimumVersion:minimumVersion,now:now,
            authorizedKeys:[Self.keyID(publicKeyBase64):publicKeyBase64]
        )
    }

    static func verify(
        envelopeData:Data,
        minimumVersion:Int64,
        now:Date=Date(),
        authorizedKeys:[String:String]
    )throws->IOSVerifiedConfig {
        guard
            let envelope = try JSONSerialization.jsonObject(with: envelopeData) as? [String: Any],
            let payloadText = envelope["payload"] as? String,
            let signatureText = envelope["signature"] as? String,
            let keyID = envelope["key_id"] as? String,
            let payload = Data(base64URLEncoded: payloadText),
            let signature = Data(base64URLEncoded: signatureText),
            let encodedKey=authorizedKeys[keyID],
            let keyData=Data(base64URLEncoded:encodedKey),
            keyData.count==32
        else{throw IOSControlError.invalidSignature}

        let key=try Curve25519.Signing.PublicKey(rawRepresentation:keyData)
        guard key.isValidSignature(signature, for: payload) else { throw IOSControlError.invalidSignature }
        guard let json = try JSONSerialization.jsonObject(with: payload) as? [String: Any],
              (json["schema_version"] as? NSNumber)?.intValue == 1,
              let versionNumber = json["version"] as? NSNumber,
              let createdText = json["created_at"] as? String,
              let expiresText = json["expires_at"] as? String
        else { throw IOSControlError.invalidResponse }

        let version=versionNumber.int64Value
        guard version>0 && version>=minimumVersion else { throw IOSControlError.rollbackDetected }

        let formatter=ISO8601DateFormatter()
        guard let created=formatter.date(from:createdText),
              let expires=formatter.date(from:expiresText),
              created <= now.addingTimeInterval(300),
              expires > now
        else { throw IOSControlError.expiredConfiguration }

        return IOSVerifiedConfig(version:version,expiresAt:expires,rawPayload:payload,json:json)
    }

    private static func keyID(_ encoded:String)->String {
        guard let raw=Data(base64URLEncoded:encoded) else{return ""}
        return SHA256.hash(data:raw).prefix(8).map{String(format:"%02x",$0)}.joined()
    }
}

private extension Data {
    init?(base64URLEncoded value: String) {
        var text = value.replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        let mod = text.count % 4
        if mod != 0 { text += String(repeating: "=", count: 4-mod) }
        self.init(base64Encoded: text)
    }
}
