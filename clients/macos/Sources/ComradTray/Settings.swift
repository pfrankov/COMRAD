import Foundation
import Security

private let settingsFileName = "comrad-tray-settings.json"
private let keychainService = "com.comrad.tray"
private let keychainTokenAccount = "worker-token"

struct Settings: Codable, Equatable {
    var managerURL: String = "http://127.0.0.1:1922"
    var slotCount: Int = 1
    var statusPort: Int = 1923
    var p2pPort: Int = 6881
    var p2pMaxUploads: Int = 8
    var p2pDownloadTimeoutSeconds: Int = 120
    var disableP2P: Bool = false
    var nodeID: String = ""
    var nodeName: String = ""
    var launchAtLogin: Bool = false

    // token is stored in Keychain, not serialized to JSON
    enum CodingKeys: String, CodingKey {
        case managerURL, slotCount, statusPort, p2pPort, p2pMaxUploads,
             p2pDownloadTimeoutSeconds, disableP2P, nodeID, nodeName, launchAtLogin
    }

    func envVars(token: String) -> [String: String] {
        var env: [String: String] = [
            "COMRAD_MANAGER_URL": managerURL,
            "COMRAD_WORKER_SLOTS": "\(slotCount)",
            "COMRAD_WORKER_STATUS_ADDR": "127.0.0.1:\(statusPort)",
            "COMRAD_WORKER_P2P_PORT": "\(p2pPort)",
            "COMRAD_WORKER_P2P_MAX_UPLOADS": "\(p2pMaxUploads)",
            "COMRAD_WORKER_P2P_DOWNLOAD_TIMEOUT_SECONDS": "\(p2pDownloadTimeoutSeconds)",
            "COMRAD_WORKER_DISABLE_P2P": disableP2P ? "true" : "false",
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

    // MARK: Keychain

    func saveToken(_ token: String) throws {
        let data = token.data(using: .utf8)!
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: keychainService,
            kSecAttrAccount as String: keychainTokenAccount,
            kSecValueData as String: data,
        ]
        SecItemDelete(query as CFDictionary)
        let status = SecItemAdd(query as CFDictionary, nil)
        if status != errSecSuccess {
            throw KeychainError.saveFailed(status)
        }
    }

    func loadToken() -> String {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: keychainService,
            kSecAttrAccount as String: keychainTokenAccount,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var result: AnyObject?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        guard status == errSecSuccess,
              let data = result as? Data,
              let token = String(data: data, encoding: .utf8) else {
            return ""
        }
        return token
    }

    enum KeychainError: Error {
        case saveFailed(OSStatus)
    }
}
