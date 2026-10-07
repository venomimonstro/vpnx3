import Foundation

final class IOSConfigStore {
    private let defaults=UserDefaults.standard

    var highestVersion:Int64 {
        get { Int64(defaults.object(forKey:"highest_config_version") as? Int ?? 0) }
        set { defaults.set(Int(newValue),forKey:"highest_config_version") }
    }

    var envelope:Data? {
        get { defaults.data(forKey:"config_envelope") }
        set { defaults.set(newValue,forKey:"config_envelope") }
    }
}
