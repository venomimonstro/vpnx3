import Foundation
import CryptoKit
import Security

final class IOSDeviceIdentity {
    static let shared = IOSDeviceIdentity()

    private let keyTag = "ru.vpnx3.device-identity-v1"
    private let state = IOSLocalState()

    func registration(controlURL: URL) async throws -> IOSRegistration {
        if let user = state.userID, let device = state.deviceID {
            return IOSRegistration(userID: user, deviceID: device)
        }

        let publicKey = try key().publicKey.x963Representation.base64URLEncodedString()
        let object: [String: Any] = [
            "platform": "ios",
            "display_name": Host.current().localizedName ?? "iPhone",
            "identity_algorithm": "ecdsa-p256-sha256",
            "public_key": publicKey
        ]
        let data = try JSONSerialization.data(withJSONObject: object)
        let path = "/api/v1/client/register"
        let response = try await signedRequest(
            controlURL: controlURL,path: path,deviceID: nil,body: data
        )
        guard
            let json = try JSONSerialization.jsonObject(with: response) as? [String: Any],
            let user = json["user_id"] as? String,
            let device = json["device_id"] as? String
        else { throw IOSControlError.invalidResponse }
        state.userID = user
        state.deviceID = device
        state.sequence = 0
        return IOSRegistration(userID: user, deviceID: device)
    }

    func signedJSON(
        controlURL: URL,
        path: String,
        deviceID: String,
        object: [String: Any]
    ) async throws -> String {
        var payload = object
        payload["sequence"] = state.reserveNextSequence()
        let body = try JSONSerialization.data(withJSONObject: payload)
        let data = try await signedRequest(
            controlURL: controlURL,path: path,deviceID: deviceID,body: body
        )
        return String(decoding: data, as: UTF8.self)
    }

    private func signedRequest(
        controlURL: URL,
        path: String,
        deviceID: String?,
        body: Data
    ) async throws -> Data {
        let timestamp = String(Int(Date().timeIntervalSince1970))
        let digest = SHA256.hash(data: body).map { String(format: "%02x", $0) }.joined()
        let canonical = ["POST",path,timestamp,digest].joined(separator: "\n")
        let signature = try key().signature(for: Data(canonical.utf8)).derRepresentation.base64URLEncodedString()

        var request = URLRequest(url: controlURL.appendingPathComponent(path))
        request.httpMethod = "POST"
        request.httpBody = body
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue(timestamp, forHTTPHeaderField: "X-VPNX3-Timestamp")
        request.setValue(signature, forHTTPHeaderField: "X-VPNX3-Signature")
        if let deviceID { request.setValue(deviceID, forHTTPHeaderField: "X-VPNX3-Device-ID") }

        let (data,response) = try await URLSession.shared.data(for: request)
        guard let http = response as? HTTPURLResponse, (200..<300).contains(http.statusCode) else {
            throw IOSControlError.invalidResponse
        }
        return data
    }

    private func key() throws -> P256.Signing.PrivateKey {
        let tagData = Data(keyTag.utf8)
        let query: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: tagData,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecReturnRef as String: true
        ]
        var item: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &item)
        if status == errSecSuccess, let secKey = item {
            let raw = SecKeyCopyExternalRepresentation(secKey as! SecKey, nil)! as Data
            return try P256.Signing.PrivateKey(rawRepresentation: raw)
        }

        let generated = P256.Signing.PrivateKey()
        let raw = generated.rawRepresentation
        let attrs: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: tagData,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecValueData as String: raw,
            kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        SecItemDelete(query as CFDictionary)
        let addStatus = SecItemAdd(attrs as CFDictionary,nil)
        guard addStatus == errSecSuccess else {
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(addStatus))
        }
        return generated
    }
}

private final class IOSLocalState {
    private let defaults = UserDefaults.standard
    var userID: String? {
        get { defaults.string(forKey: "user_id") }
        set { defaults.set(newValue, forKey: "user_id") }
    }
    var deviceID: String? {
        get { defaults.string(forKey: "device_id") }
        set { defaults.set(newValue, forKey: "device_id") }
    }
    var sequence: Int {
        get { defaults.integer(forKey: "request_sequence") }
        set { defaults.set(newValue, forKey: "request_sequence") }
    }
    func reserveNextSequence() -> Int {
        sequence += 1
        return sequence
    }
}

private extension Data {
    func base64URLEncodedString() -> String {
        base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }
}
