import Foundation

final class IOSTrustRepository {
    private let runtime=IOSRuntimeConfig.current
    private let store=IOSTrustStore()
    private let configStore=IOSConfigStore()

    func refreshOrFallback(now:Date=Date()) async throws->IOSVerifiedTrustBundle {
        guard !runtime.trustRootPublicKey.isEmpty else{throw IOSControlError.runtimeNotConfigured}
        let verifier=IOSTrustBundleVerifier(rootPublicKey:runtime.trustRootPublicKey)
        let minimum=store.highestVersion
        var sources:[URL]=[]
        if let u=URL(string:"/api/v1/trust/bundle",relativeTo:runtime.controlURL)?.absoluteURL{sources.append(u)}
        for configURL in runtime.configBootstrapURLs+configStore.mirrorURLs {
            guard var c=URLComponents(url:configURL,resolvingAgainstBaseURL:false) else{continue}
            c.path="/api/v1/trust/bundle";c.query=nil;c.fragment=nil
            if let u=c.url{sources.append(u)}
        }
        var last:Error=IOSControlError.invalidResponse
        var seen=Set<String>()
        for url in sources where seen.insert(url.absoluteString).inserted {
            guard url.scheme=="https",url.host != nil,url.user==nil,url.fragment==nil else{continue}
            do{
                var request=URLRequest(url:url);request.timeoutInterval=10;request.cachePolicy=.reloadIgnoringLocalCacheData
                request.setValue("application/json",forHTTPHeaderField:"Accept")
                let (data,response)=try await URLSession.shared.data(for:request)
                guard (response as? HTTPURLResponse)?.statusCode==200 else{throw IOSControlError.invalidResponse}
                let verified=try verifier.verify(data,minimumVersion:minimum,now:now)
                store.envelope=data;store.highestVersion=max(minimum,verified.version)
                return verified
            }catch{last=error}
        }
        guard let cached=store.envelope else{throw last}
        return try verifier.verify(cached,minimumVersion:store.highestVersion,now:now)
    }
}
