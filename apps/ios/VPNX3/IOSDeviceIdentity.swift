import Foundation
import Security
import UIKit
import CryptoKit

final class IOSDeviceIdentity {
    static let shared = IOSDeviceIdentity()

    private let keyTag = Data("ru.vpnx3.device-identity-v1".utf8)
    private let state = IOSLocalState()

    func registration(controlURL: URL) async throws -> IOSRegistration {
        if let user = state.userID, let device = state.deviceID {
            return IOSRegistration(userID: user, deviceID: device)
        }

        let publicKey = try publicKeySPKI().base64URLEncodedString()
        let object: [String: Any] = [
            "platform": "ios",
            "display_name": UIDevice.current.name,
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
        state.resetSequence()
        return IOSRegistration(userID: user, deviceID: device)
    }

    func signedJSON(
        controlURL: URL,
        path: String,
        deviceID: String,
        object: [String: Any]
    ) async throws -> String {
        var payload = object
        payload["sequence"] = try state.reserveNextSequence()
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
        let digest = SHA256Digest.hex(body)
        let canonical = Data(["POST",path,timestamp,digest].joined(separator: "\n").utf8)
        let signature = try sign(canonical).base64URLEncodedString()

        guard let url=URL(string:path,relativeTo:controlURL)?.absoluteURL else {
            throw IOSControlError.runtimeNotConfigured
        }
        var request = URLRequest(url: url)
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

    private func privateKey() throws -> SecKey {
        let query: [String: Any] = [
            kSecClass as String:kSecClassKey,
            kSecAttrApplicationTag as String:keyTag,
            kSecAttrKeyType as String:kSecAttrKeyTypeECSECPrimeRandom,
            kSecReturnRef as String:true
        ]
        var item:CFTypeRef?
        let status=SecItemCopyMatching(query as CFDictionary,&item)
        if status==errSecSuccess,let item { return (item as! SecKey) }
        if status != errSecItemNotFound {
            throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))
        }

        let attributes: [String: Any] = [
            kSecAttrKeyType as String:kSecAttrKeyTypeECSECPrimeRandom,
            kSecAttrKeySizeInBits as String:256,
            kSecPrivateKeyAttrs as String:[
                kSecAttrIsPermanent as String:true,
                kSecAttrApplicationTag as String:keyTag,
                kSecAttrAccessible as String:kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            ]
        ]
        var error:Unmanaged<CFError>?
        guard let key=SecKeyCreateRandomKey(attributes as CFDictionary,&error) else {
            throw error!.takeRetainedValue() as Error
        }
        return key
    }

    private func publicKeySPKI() throws -> Data {
        let privateKey=try privateKey()
        guard let publicKey=SecKeyCopyPublicKey(privateKey) else { throw IOSControlError.invalidResponse }
        var error:Unmanaged<CFError>?
        guard let x963=SecKeyCopyExternalRepresentation(publicKey,&error) as Data? else {
            throw error!.takeRetainedValue() as Error
        }
        guard x963.count==65 && x963.first==0x04 else { throw IOSControlError.invalidResponse }

        // DER SubjectPublicKeyInfo for id-ecPublicKey + prime256v1 followed by
        // a 65-byte uncompressed P-256 point.
        let prefix=Data([
            0x30,0x59,0x30,0x13,0x06,0x07,0x2a,0x86,0x48,0xce,0x3d,0x02,0x01,
            0x06,0x08,0x2a,0x86,0x48,0xce,0x3d,0x03,0x01,0x07,
            0x03,0x42,0x00
        ])
        return prefix+x963
    }

    private func sign(_ message:Data)throws->Data{
        let key=try privateKey()
        let algorithm=SecKeyAlgorithm.ecdsaSignatureMessageX962SHA256
        guard SecKeyIsAlgorithmSupported(key,.sign,algorithm) else { throw IOSControlError.unsupported }
        var error:Unmanaged<CFError>?
        guard let sig=SecKeyCreateSignature(key,algorithm,message as CFData,&error) as Data? else {
            throw error!.takeRetainedValue() as Error
        }
        return sig
    }
}

private final class IOSLocalState {
    private let defaults = UserDefaults.standard
    private let sequenceService="ru.vpnx3.device-sequence"
    private let sequenceAccount="request-sequence-v1"

    var userID: String? {
        get { defaults.string(forKey: "user_id") }
        set { defaults.set(newValue, forKey: "user_id") }
    }
    var deviceID: String? {
        get { defaults.string(forKey: "device_id") }
        set { defaults.set(newValue, forKey: "device_id") }
    }

    func resetSequence() {
        try? writeSequence(0)
    }

    func reserveNextSequence() throws -> Int64 {
        let next=try readSequence()+1
        guard next>0 else { throw IOSControlError.invalidResponse }
        try writeSequence(next)
        return next
    }

    private func readSequence() throws -> Int64 {
        let query:[String:Any]=[
            kSecClass as String:kSecClassGenericPassword,
            kSecAttrService as String:sequenceService,
            kSecAttrAccount as String:sequenceAccount,
            kSecReturnData as String:true,
            kSecMatchLimit as String:kSecMatchLimitOne
        ]
        var item:CFTypeRef?
        let status=SecItemCopyMatching(query as CFDictionary,&item)
        if status==errSecItemNotFound{return 0}
        guard status==errSecSuccess,let data=item as? Data,data.count==8 else {
            throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))
        }
        return data.reduce(Int64(0)) { ($0 << 8) | Int64($1) }
    }

    private func writeSequence(_ value:Int64)throws{
        var big=value.bigEndian
        let data=Data(bytes:&big,count:8)
        let query:[String:Any]=[
            kSecClass as String:kSecClassGenericPassword,
            kSecAttrService as String:sequenceService,
            kSecAttrAccount as String:sequenceAccount
        ]
        let attrs:[String:Any]=[
            kSecValueData as String:data,
            kSecAttrAccessible as String:kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        let status=SecItemUpdate(query as CFDictionary,attrs as CFDictionary)
        if status==errSecItemNotFound{
            var add=query
            attrs.forEach{add[$0.key]=$0.value}
            let addStatus=SecItemAdd(add as CFDictionary,nil)
            guard addStatus==errSecSuccess else { throw NSError(domain:NSOSStatusErrorDomain,code:Int(addStatus)) }
        }else if status != errSecSuccess{
            throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))
        }
    }
}

private enum SHA256Digest {
    static func hex(_ data:Data)->String {
        SHA256.hash(data:data).map{String(format:"%02x",$0)}.joined()
    }
}

private extension Data {
    func base64URLEncodedString() -> String {
        base64EncodedString()
            .replacingOccurrences(of:"+",with:"-")
            .replacingOccurrences(of:"/",with:"_")
            .replacingOccurrences(of:"=",with:"")
    }
}
