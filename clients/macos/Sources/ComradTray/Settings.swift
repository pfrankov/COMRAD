import Foundation

private let settingsFileName = "comrad-tray-settings.json"

struct Settings: Codable, Equatable {
    var managerURL: String = "http://127.0.0.1:1922"
    var statusPort: Int = 1923
    var p2pPort: Int = 6881
    var p2pMaxUploads: Int = 8
    var p2pDownloadTimeoutSeconds: Int = 120
    var disableP2P: Bool = false
    var nodeID: String = ""
    var nodeName: String = ""
    var launchAtLogin: Bool = false
    var memoryGB: Int = Settings.physicalMemoryGB()
    var diskGB: Int = Settings.availableDiskGB()
    var idleOnlyMode: Bool = false
    var token: String = ""
    var language: String = ""

    func envVars() -> [String: String] {
        let appSupport = FileManager.default
            .urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        let dataDir = appSupport.appendingPathComponent("COMRAD/data")
        var env: [String: String] = [
            "COMRAD_MANAGER_URL": managerURL,
            "COMRAD_WORKER_SLOTS": "1",
            "COMRAD_WORKER_STATUS_ADDR": "127.0.0.1:\(statusPort)",
            "COMRAD_WORKER_P2P_PORT": "\(p2pPort)",
            "COMRAD_WORKER_P2P_MAX_UPLOADS": "\(p2pMaxUploads)",
            "COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS": "\(p2pDownloadTimeoutSeconds)",
            "COMRAD_WORKER_DISABLE_P2P": disableP2P ? "true" : "false",
            "COMRAD_WORKER_STATE_PATH": dataDir.appendingPathComponent("worker-state.json").path,
            "COMRAD_WORKER_CACHE_DIR": dataDir.appendingPathComponent("worker-cache").path,
            "COMRAD_WORKER_UNIFIED_BYTES": "\(Int64(memoryGB) * (1 << 30))",
            "COMRAD_WORKER_RAM_BYTES": "\(Int64(memoryGB) * (1 << 30))",
            "COMRAD_WORKER_DISK_BYTES": "\(Int64(diskGB) * (1 << 30))",
        ]
        if !token.isEmpty {
            env["COMRAD_WORKER_TOKEN"] = token
        }
        if !nodeID.isEmpty {
            env["COMRAD_NODE_ID"] = nodeID
        }
        if !nodeName.isEmpty {
            env["COMRAD_NODE_NAME"] = nodeName
        }
        return env
    }
}

final class SettingsStore {
    let fileURL: URL

    init(fileURL: URL? = nil) {
        if let url = fileURL {
            self.fileURL = url
        } else {
            let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
            let dir = appSupport.appendingPathComponent("COMRAD", isDirectory: true)
            try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            self.fileURL = dir.appendingPathComponent(settingsFileName)
        }
    }

    func load() -> Settings {
        guard let data = try? Data(contentsOf: fileURL),
              let settings = try? JSONDecoder().decode(Settings.self, from: data) else {
            return Settings()
        }
        return settings
    }

    func save(_ settings: Settings) throws {
        let data = try JSONEncoder().encode(settings)
        try data.write(to: fileURL, options: .atomic)
    }
}

extension Settings {
    static func physicalMemoryGB() -> Int {
        Int(ProcessInfo.processInfo.physicalMemory / (1 << 30))
    }

    static func availableDiskGB() -> Int {
        let url = URL(fileURLWithPath: NSHomeDirectory())
        let values = try? url.resourceValues(forKeys: [.volumeAvailableCapacityForImportantUsageKey])
        let bytes = values?.volumeAvailableCapacityForImportantUsage ?? Int64(20 << 30)
        return Int(bytes / (1 << 30))
    }
}
