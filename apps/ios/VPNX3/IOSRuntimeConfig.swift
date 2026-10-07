import Foundation

struct IOSRuntimeConfig {
    let controlURL: URL
    let configPublicKey: String

    static let current: IOSRuntimeConfig = {
        let control = Bundle.main.object(forInfoDictionaryKey: "VPNX3ControlURL") as? String ?? ""
        let key = Bundle.main.object(forInfoDictionaryKey: "VPNX3ConfigPublicKey") as? String ?? ""
        return IOSRuntimeConfig(
            controlURL: URL(string: control) ?? URL(string: "https://invalid.invalid")!,
            configPublicKey: key
        )
    }()
}
