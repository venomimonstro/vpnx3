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

    func verify(envelopeData: Data, minimumVersion: Int64, now: Date = Date()) throws -> IOSVerifiedConfig {
        guard
            let envelope = try JSONSerialization.jsonObject(with: envelopeData) as? [String: Any],
            let payloadText = envelope["payload"] as? String,
            let signatureText = envelope["signature"] as? String,
            let keyID = envelope["key_id"] as? String,
            let payload = Data(base64URLEncoded: payloadText),
            let signature = Data(base64URLEncoded: signatureText),
            let keyData = Data(base64URLEncoded: publicKeyBase64),
            keyData.count == 32
        else { throw IOSControlError.invalidResponse }

        let expectedKeyID = SHA256.hash(data:keyData).prefix(8).map { String(format:"%02x",$0) }.joined()
        guard keyID == expectedKeyID else { throw IOSControlError.invalidSignature }

        let key = try Curve25519.Signing.PublicKey(rawRepresentation: keyData)
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
