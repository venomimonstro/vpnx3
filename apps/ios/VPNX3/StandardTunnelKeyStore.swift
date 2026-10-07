import Foundation
import Security
import WireGuardKit

final class StandardTunnelKeyStore {
    private let service = "ru.vpnx3.standard-wireguard"
    private let account = "private-key-v1"

    func ensure() throws -> PrivateKey {
        if let existing = try read(),
           let text = String(data: existing, encoding: .ascii),
           let key = PrivateKey(base64Key: text) {
            return key
        }
        let key = PrivateKey()
        try save(Data(key.base64Key.utf8))
        return key
    }

    private func save(_ data: Data) throws {
        let query: [String: Any] = [
            kSecClass as String:kSecClassGenericPassword,
            kSecAttrService as String:service,
            kSecAttrAccount as String:account
        ]
        SecItemDelete(query as CFDictionary)
        var add = query
        add[kSecValueData as String] = data
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        let status = SecItemAdd(add as CFDictionary,nil)
        guard status == errSecSuccess else { throw NSError(domain:NSOSStatusErrorDomain,code:Int(status)) }
    }

    private func read() throws -> Data? {
        let query: [String: Any] = [
            kSecClass as String:kSecClassGenericPassword,
            kSecAttrService as String:service,
            kSecAttrAccount as String:account,
            kSecReturnData as String:true,
            kSecMatchLimit as String:kSecMatchLimitOne
        ]
        var result:CFTypeRef?
        let status=SecItemCopyMatching(query as CFDictionary,&result)
        if status==errSecItemNotFound{return nil}
        guard status==errSecSuccess else { throw NSError(domain:NSOSStatusErrorDomain,code:Int(status)) }
        return result as? Data
    }
}
