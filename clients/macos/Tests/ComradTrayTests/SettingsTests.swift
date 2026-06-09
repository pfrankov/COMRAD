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
        s.statusPort = 2000
        s.p2pPort = 7000
        s.disableP2P = true
        s.nodeID = "node-test"
        s.nodeName = "test-machine"
        s.idleOnlyMode = true
        s.token = "round-trip-token"

        try store.save(s)
        let loaded = store.load()

        XCTAssertEqual(loaded.managerURL, s.managerURL)
        XCTAssertEqual(loaded.statusPort, s.statusPort)
        XCTAssertEqual(loaded.p2pPort, s.p2pPort)
        XCTAssertEqual(loaded.disableP2P, s.disableP2P)
        XCTAssertEqual(loaded.nodeID, s.nodeID)
        XCTAssertEqual(loaded.nodeName, s.nodeName)
        XCTAssertEqual(loaded.idleOnlyMode, s.idleOnlyMode)
        XCTAssertEqual(loaded.token, s.token)
    }

    func testDefaultValuesAreSane() {
        let s = Settings()
        XCTAssertFalse(s.managerURL.isEmpty)
        XCTAssertGreaterThan(s.statusPort, 0)
        XCTAssertFalse(s.disableP2P)
        XCTAssertFalse(s.idleOnlyMode)
        XCTAssertTrue(s.token.isEmpty)
    }

    func testIdleOnlyModeDefaultsToFalse() {
        // Confirm that settings loaded from JSON without the field default to false.
        let json = """
        {"nodeID":"","p2pDownloadTimeoutSeconds":120,"memoryGB":16,"statusPort":1923,"p2pPort":6881,
         "p2pMaxUploads":8,"managerURL":"http://127.0.0.1:1922","nodeName":"","disableP2P":false,
         "diskGB":100,"launchAtLogin":false}
        """.data(using: .utf8)!
        let url = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("comrad-legacy-\(UUID().uuidString).json")
        try? json.write(to: url)
        let loaded = SettingsStore(fileURL: url).load()
        XCTAssertFalse(loaded.idleOnlyMode, "idleOnlyMode must default to false for old settings files")
        XCTAssertTrue(loaded.token.isEmpty, "token must default to empty for old settings files")
    }

    func testTokenRoundTrip() throws {
        let store = tempStore()
        var s = Settings()
        s.token = "secret-token-\(UUID().uuidString)"
        try store.save(s)
        let loaded = store.load()
        XCTAssertEqual(loaded.token, s.token)
    }

    func testEnvVarsMapsAllSettings() {
        var s = Settings()
        s.managerURL = "http://manager:9000"
        s.statusPort = 1923
        s.p2pPort = 6882
        s.disableP2P = true
        s.nodeID = "n1"
        s.nodeName = "my-machine"
        s.token = "secret-token"

        let env = s.envVars()

        XCTAssertEqual(env["COMRAD_MANAGER_URL"], "http://manager:9000")
        XCTAssertEqual(env["COMRAD_WORKER_SLOTS"], "1")
        XCTAssertEqual(env["COMRAD_WORKER_STATUS_ADDR"], "127.0.0.1:1923")
        XCTAssertEqual(env["COMRAD_WORKER_P2P_PORT"], "6882")
        XCTAssertEqual(env["COMRAD_WORKER_DISABLE_P2P"], "true")
        XCTAssertEqual(env["COMRAD_WORKER_TOKEN"], "secret-token")
        XCTAssertEqual(env["COMRAD_NODE_ID"], "n1")
        XCTAssertEqual(env["COMRAD_NODE_NAME"], "my-machine")
    }

    func testEnvVarsIncludesResourceBytes() {
        var s = Settings()
        s.memoryGB = 8
        s.diskGB = 100
        let env = s.envVars()
        XCTAssertEqual(env["COMRAD_WORKER_UNIFIED_BYTES"], "\(8 * (1 << 30))")
        XCTAssertEqual(env["COMRAD_WORKER_RAM_BYTES"],     "\(8 * (1 << 30))")
        XCTAssertEqual(env["COMRAD_WORKER_DISK_BYTES"],    "\(100 * (1 << 30))")
    }

    func testMissingJSONFallsBackToDefaults() {
        let store = SettingsStore(fileURL: URL(fileURLWithPath: "/nonexistent/path/settings.json"))
        let s = store.load()
        XCTAssertFalse(s.managerURL.isEmpty)
        XCTAssertGreaterThan(s.statusPort, 0)
    }
}
