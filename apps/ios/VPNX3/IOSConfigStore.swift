import Foundation
import Security

final class IOSConfigStore {
    private let service="ru.vpnx3.config-state-v1"

    var highestVersion:Int64 {
        get { (try? readInt64("highest_version")) ?? 0 }
        set { try? writeInt64("highest_version",newValue) }
    }

    var envelope:Data? {
        get { try? readData("envelope") }
        set { try? writeData("envelope",newValue) }
    }

    var mirrorURLs:[URL] {
        get {
            guard let data=try? readData("mirror_urls"),let data else{return []}
            return (try? JSONDecoder().decode([String].self,from:data))?
                .compactMap(URL.init(string:)) ?? []
        }
        set {
            let values=Array(newValue.map(\.absoluteString).uniqued().prefix(16))
            let data=try? JSONEncoder().encode(values)
            try? writeData("mirror_urls",data)
        }
    }

    private func query(_ account:String)->[String:Any] {
        [
            kSecClass as String:kSecClassGenericPassword,
            kSecAttrService as String:service,
            kSecAttrAccount as String:account
        ]
    }

    private func readData(_ account:String)throws->Data?{
        var q=query(account)
        q[kSecReturnData as String]=true
        q[kSecMatchLimit as String]=kSecMatchLimitOne
        var item:CFTypeRef?
        let status=SecItemCopyMatching(q as CFDictionary,&item)
        if status==errSecItemNotFound{return nil}
        guard status==errSecSuccess else{throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))}
        return item as? Data
    }

    private func writeData(_ account:String,_ data:Data?)throws{
        let q=query(account)
        if data==nil{
            let status=SecItemDelete(q as CFDictionary)
            if status != errSecSuccess && status != errSecItemNotFound{
                throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))
            }
            return
        }
        let attrs:[String:Any]=[
            kSecValueData as String:data!,
            kSecAttrAccessible as String:kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        ]
        let status=SecItemUpdate(q as CFDictionary,attrs as CFDictionary)
        if status==errSecItemNotFound{
            var add=q
            attrs.forEach{add[$0.key]=$0.value}
            let addStatus=SecItemAdd(add as CFDictionary,nil)
            guard addStatus==errSecSuccess else{throw NSError(domain:NSOSStatusErrorDomain,code:Int(addStatus))}
        }else if status != errSecSuccess{
            throw NSError(domain:NSOSStatusErrorDomain,code:Int(status))
        }
    }

    private func readInt64(_ account:String)throws->Int64{
        guard let data=try readData(account) else{return 0}
        guard data.count==8 else{throw IOSControlError.invalidResponse}
        return data.reduce(Int64(0)){($0<<8)|Int64($1)}
    }

    private func writeInt64(_ account:String,_ value:Int64)throws{
        var big=value.bigEndian
        try writeData(account,Data(bytes:&big,count:8))
    }
}


private extension Array where Element==String {
    func uniqued()->[String]{
        var seen=Set<String>()
        return filter{seen.insert($0).inserted}
    }
}
