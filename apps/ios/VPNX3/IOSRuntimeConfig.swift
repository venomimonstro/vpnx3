import Foundation

struct IOSRuntimeConfig {
    let controlURL: URL
    let configPublicKey: String
    let releasePublicKey: String
    let configBootstrapURLs: [URL]

    static let current: IOSRuntimeConfig = {
        let control = Bundle.main.object(forInfoDictionaryKey: "VPNX3ControlURL") as? String ?? ""
        let key = Bundle.main.object(forInfoDictionaryKey: "VPNX3ConfigPublicKey") as? String ?? ""
        let releaseKey = Bundle.main.object(forInfoDictionaryKey: "VPNX3ReleasePublicKey") as? String ?? ""
        let bootstrapRaw = Bundle.main.object(forInfoDictionaryKey: "VPNX3ConfigBootstrapURLs") as? String ?? ""
        let bootstraps = bootstrapRaw
            .split(whereSeparator: { $0 == "," || $0 == ";" })
            .compactMap { URL(string:String($0).trimmingCharacters(in:.whitespacesAndNewlines)) }
            .filter { $0.scheme=="https" && $0.host != nil && $0.user == nil && $0.fragment == nil }
        return IOSRuntimeConfig(
            controlURL: URL(string: control) ?? URL(string: "https://invalid.invalid")!,
            configPublicKey: key,
            releasePublicKey: releaseKey,
            configBootstrapURLs: Array(bootstraps.prefix(16))
        )
    }()
}
