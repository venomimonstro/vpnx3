import Foundation
import CryptoKit

struct IOSConfigVerifier {
    let publicKeyBase64: String

    func verify(envelopeData: Data) throws -> IOSVerifiedConfig {
        guard
            let envelope = try JSONSerialization.jsonObject(with: envelopeData) as? [String: Any],
            let payloadText = envelope["payload"] as? String,
            let signatureText = envelope["signature"] as? String,
            let payload = Data(base64URLEncoded: payloadText),
            let signature = Data(base64URLEncoded: signatureText),
            let keyData = Data(base64URLEncoded: publicKeyBase64)
        else { throw IOSControlError.invalidResponse }

        let key = try Curve25519.Signing.PublicKey(rawRepresentation: keyData)
        guard key.isValidSignature(signature, for: payload) else { throw IOSControlError.invalidSignature }
        guard let json = try JSONSerialization.jsonObject(with: payload) as? [String: Any] else {
            throw IOSControlError.invalidResponse
        }
        return IOSVerifiedConfig(rawPayload: payload, json: json)
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
