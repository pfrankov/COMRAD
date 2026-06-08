import XCTest
@testable import ComradTray

final class StatusPollerTests: XCTestCase {

    func testDecodesValidSnapshot() throws {
        let json = """
        {
          "connected": true,
          "nodeId": "node-abc",
          "nodeName": "test-machine",
          "version": "1.2.3",
          "target": "darwin-arm64-metal",
          "runtimeAdapters": ["llama.cpp-metal"],
          "slots": [{"id": "node-abc/metal0", "state": "idle"}],
          "cachedCount": 2,
          "warmCount": 1,
          "warmProfiles": ["profile-x"],
          "managerUrl": "http://manager:1922",
          "startedAt": "2026-06-01T00:00:00Z",
          "updatedAt": "2026-06-08T00:00:00Z"
        }
        """.data(using: .utf8)!

        let snap = try JSONDecoder().decode(WorkerStatusSnapshot.self, from: json)
        XCTAssertTrue(snap.connected)
        XCTAssertEqual(snap.nodeId, "node-abc")
        XCTAssertEqual(snap.slots.count, 1)
        XCTAssertEqual(snap.slots[0].state, "idle")
        XCTAssertEqual(snap.cachedCount, 2)
        XCTAssertNil(snap.lastError)
        XCTAssertNil(snap.p2p)
    }

    func testDecodesSnapshotWithOptionalP2PAndError() throws {
        let json = """
        {
          "connected": false,
          "nodeId": "node-x",
          "nodeName": "",
          "version": "dev",
          "target": "darwin-arm64-metal",
          "runtimeAdapters": [],
          "slots": [],
          "cachedCount": 0,
          "warmCount": 0,
          "warmProfiles": [],
          "managerUrl": "http://127.0.0.1:1922",
          "lastError": "dial tcp: connection refused",
          "p2p": {"available": true, "port": 6881, "peers": 3},
          "startedAt": "2026-06-08T00:00:00Z",
          "updatedAt": "2026-06-08T00:00:01Z"
        }
        """.data(using: .utf8)!

        let snap = try JSONDecoder().decode(WorkerStatusSnapshot.self, from: json)
        XCTAssertFalse(snap.connected)
        XCTAssertEqual(snap.lastError, "dial tcp: connection refused")
        XCTAssertNotNil(snap.p2p)
        XCTAssertEqual(snap.p2p?.port, 6881)
        XCTAssertEqual(snap.p2p?.peers, 3)
    }

    func testDecodeFailsGracefullyOnBadJSON() {
        let badJSON = "not json at all".data(using: .utf8)!
        XCTAssertThrowsError(try JSONDecoder().decode(WorkerStatusSnapshot.self, from: badJSON))
    }
}
