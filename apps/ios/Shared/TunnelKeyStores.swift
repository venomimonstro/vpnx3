import Foundation
import Security
import WireGuardKit

private enum VPNX3SharedKeychain {
    static var accessGroup:String {
        guard let value=Bundle.main.object(forInfoDictionaryKey:"VPNX3KeychainAccessGroup") as? String,
              !value.isEmpty else{
            fatalError("VPNX3KeychainAccessGroup is not configured")
        }
        return value
    }

    static func query(service:String,account:String)->[String:Any]{
        [
            kSecClass as String:kSecClassGenericPassword,
            kSecAttrService as String:service,
            kSecAttrAccount as String:account,
            kSecAttrAccessGroup as String:accessGroup,
            kSecAttrSynchronizable as String:false
        ]
    }

    static func read(service:String,account:String)throws->Data?{
        var q=query(service:service,account:account)
        q[kSecReturnData as String]=true
        q[kSecMatchLimit as String]=kSecMatchLimitOne
        var item:CFTypeRef?
        let status=SecItemCopyMatching(q as CFDictionary,&item)
        if status==errSecItemNotFound{return nil}
        guard status==errSecSuccess else{
            throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))
        }
        return item as? Data
    }

    static func write(service:String,account:String,data:Data)throws{
        let q=query(service:service,account:account)
        let attrs:[String:Any]=[
            kSecValueData as String:data,
            kSecAttrAccessible as String:kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        let status=SecItemUpdate(q as CFDictionary,attrs as CFDictionary)
        if status==errSecItemNotFound{
            var add=q
            attrs.forEach{add[$0.key]=$0.value}
            let addStatus=SecItemAdd(add as CFDictionary,nil)
            guard addStatus==errSecSuccess else{
                throw NSError(domain:NSOSStatusErrorDomain,code:Int(addStatus))
            }
        }else if status != errSecSuccess{
            throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))
        }
    }

    static func delete(service:String,account:String)throws{
        let status=SecItemDelete(query(service:service,account:account) as CFDictionary)
        if status != errSecSuccess && status != errSecItemNotFound{
            throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))
        }
    }
}

final class PersonalKeyStore {
    private let service="ru.vpnx3.personal-wireguard"
    private let account="private-key-v2-shared"

    struct KeyInfo {
        let privateKey:PrivateKey
        var publicKey:PublicKey{privateKey.publicKey}
    }

    func ensure()throws->KeyInfo{
        if let data=try VPNX3SharedKeychain.read(service:service,account:account),
           let text=String(data:data,encoding:.ascii),
           let key=PrivateKey(base64Key:text){
            return KeyInfo(privateKey:key)
        }
        let key=PrivateKey()
        try VPNX3SharedKeychain.write(
            service:service,account:account,data:Data(key.base64Key.utf8)
        )
        return KeyInfo(privateKey:key)
    }

    func rotate()throws->KeyInfo{
        try delete()
        return try ensure()
    }

    func delete()throws{
        try VPNX3SharedKeychain.delete(service:service,account:account)
    }
}

final class StandardTunnelKeyStore {
    private let service="ru.vpnx3.standard-wireguard"
    private let account="private-key-v2-shared"

    func ensure()throws->PrivateKey{
        if let data=try VPNX3SharedKeychain.read(service:service,account:account),
           let text=String(data:data,encoding:.ascii),
           let key=PrivateKey(base64Key:text){
            return key
        }
        let key=PrivateKey()
        try VPNX3SharedKeychain.write(
            service:service,account:account,data:Data(key.base64Key.utf8)
        )
        return key
    }
}
