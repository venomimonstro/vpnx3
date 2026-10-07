import Foundation
import Security
import WireGuardKit

final class PersonalKeyStore {
    private let service = "ru.vpnx3.personal-wireguard"
    private let account = "private-key-v1"

    struct KeyInfo {
        let privateKey: PrivateKey
        var publicKey: PublicKey { privateKey.publicKey }
    }

    func ensure() throws -> KeyInfo {
        if let data = try read() {
            guard let text = String(data: data, encoding: .ascii),
                  let key = PrivateKey(base64Key: text) else {
                try delete()
                return try create()
            }
            return KeyInfo(privateKey: key)
        }
        return try create()
    }

    func rotate() throws -> KeyInfo {
        try delete()
        return try ensure()
    }

    func delete() throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
        let status = SecItemDelete(query as CFDictionary)
        if status != errSecSuccess && status != errSecItemNotFound {
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(status))
        }
    }

    private func create() throws -> KeyInfo {
        let key = PrivateKey()
        let data = Data(key.base64Key.utf8)
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecValueData as String: data,
            kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        ]
        let status = SecItemAdd(query as CFDictionary, nil)
        guard status == errSecSuccess else {
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(status))
        }
        return KeyInfo(privateKey: key)
    }

    private func read() throws -> Data? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne
        ]
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess else {
            throw NSError(domain: NSOSStatusErrorDomain, code: Int(status))
        }
        return result as? Data
    }
}
