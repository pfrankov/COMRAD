import XCTest
@testable import ComradTray

final class SettingsTests: XCTestCase {

    private func tempStore() -> SettingsStore {
        let url = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("comrad-test-\(UUID().uuidString).json")
        return SettingsStore(fileURL: url)
    }

    func testEncodeDecodeRoundTrip() throws {
        let store = tempStore()
        var s = Settings()
        s.managerURL = "http://manager.example.com:9000"
        s.slotCount = 4
        s.statusPort = 2000
        s.p2pPort = 7000
        s.disableP2P = true
        s.nodeID = "node-test"
        s.nodeName = "test-machine"

        try store.save(s)
        let loaded = store.load()

        XCTAssertEqual(loaded.managerURL, s.managerURL)
        XCTAssertEqual(loaded.slotCount, s.slotCount)
        XCTAssertEqual(loaded.statusPort, s.statusPort)
        XCTAssertEqual(loaded.p2pPort, s.p2pPort)
        XCTAssertEqual(loaded.disableP2P, s.disableP2P)
        XCTAssertEqual(loaded.nodeID, s.nodeID)
        XCTAssertEqual(loaded.nodeName, s.nodeName)
    }

    func testDefaultValuesAreSane() {
        let s = Settings()
        XCTAssertFalse(s.managerURL.isEmpty)
        XCTAssertGreaterThan(s.slotCount, 0)
        XCTAssertGreaterThan(s.statusPort, 0)
        XCTAssertFalse(s.disableP2P)
    }

    func testTokenNotInSerializedJSON() throws {
        let store = tempStore()
        let s = Settings()
        try store.save(s)
        let fileURL = store.fileURL
        let data = try Data(contentsOf: fileURL)
        let json = String(data: data, encoding: .utf8) ?? ""
        XCTAssertFalse(json.contains("token"), "token must not appear in settings JSON: \(json)")
    }

    func testEnvVarsMapsAllSettings() {
        var s = Settings()
        s.managerURL = "http://manager:9000"
        s.slotCount = 3
        s.statusPort = 1923
        s.p2pPort = 6882
        s.disableP2P = true
        s.nodeID = "n1"
        s.nodeName = "my-machine"
        let token = "secret-token"

        let env = s.envVars(token: token)

        XCTAssertEqual(env["COMRAD_MANAGER_URL"], "http://manager:9000")
        XCTAssertEqual(env["COMRAD_WORKER_SLOTS"], "3")
        XCTAssertEqual(env["COMRAD_WORKER_STATUS_ADDR"], "127.0.0.1:1923")
        XCTAssertEqual(env["COMRAD_WORKER_P2P_PORT"], "6882")
        XCTAssertEqual(env["COMRAD_WORKER_DISABLE_P2P"], "true")
        XCTAssertEqual(env["COMRAD_WORKER_TOKEN"], "secret-token")
        XCTAssertEqual(env["COMRAD_NODE_ID"], "n1")
        XCTAssertEqual(env["COMRAD_NODE_NAME"], "my-machine")
    }

    func testMissingJSONFallsBackToDefaults() {
        let store = SettingsStore(fileURL: URL(fileURLWithPath: "/nonexistent/path/settings.json"))
        let s = store.load()
        XCTAssertFalse(s.managerURL.isEmpty)
        XCTAssertGreaterThan(s.statusPort, 0)
    }

    func testKeychainRoundTrip() throws {
        let store = tempStore()
        let originalToken = "test-keychain-token-\(UUID().uuidString)"
        try store.saveToken(originalToken)
        let loaded = store.loadToken()
        XCTAssertEqual(loaded, originalToken)
    }
}

