import XCTest
@testable import ComradTray

final class WorkerProcessTests: XCTestCase {

    func testEnvVarsMapsAllRequiredKeys() {
        var s = Settings()
        s.managerURL = "http://manager:9000"
        s.statusPort = 1923
        s.p2pPort = 6881
        s.disableP2P = false
        s.token = "tok"

        let env = s.envVars()

        let required = [
            "COMRAD_MANAGER_URL",
            "COMRAD_WORKER_SLOTS",
            "COMRAD_WORKER_STATUS_ADDR",
            "COMRAD_WORKER_P2P_PORT",
            "COMRAD_WORKER_DISABLE_P2P",
            "COMRAD_WORKER_TOKEN",
        ]
        for key in required {
            XCTAssertNotNil(env[key], "missing env key: \(key)")
        }
    }

    func testBackoffSequence() {
        let expected = [1.0, 2.0, 4.0, 8.0, 16.0, 30.0, 30.0]
        for (i, want) in expected.enumerated() {
            XCTAssertEqual(workerBackoff(attempt: i), want, "backoff at attempt \(i)")
        }
    }

    @MainActor
    func testStateMachineIdleToRunning() throws {
        // Use sleep to keep the process alive long enough to observe running state
        let binURL = URL(fileURLWithPath: "/bin/sleep")
        let dataDir = URL(fileURLWithPath: NSTemporaryDirectory())
        let proc = WorkerProcess(workerBinaryURL: binURL, dataDir: dataDir)

        var states: [WorkerProcessState] = []
        let expRunning = expectation(description: "running state")
        var fulfilled = false
        proc.onStateChange = { state in
            states.append(state)
            if case .running = state, !fulfilled {
                fulfilled = true
                expRunning.fulfill()
            }
        }
        proc.start(env: [:])

        wait(for: [expRunning], timeout: 5)
        proc.stop()

        XCTAssertTrue(states.contains(.starting), "expected .starting in \(states)")
        let hasRunning = states.contains { if case .running = $0 { return true }; return false }
        XCTAssertTrue(hasRunning, "expected .running in \(states)")
    }

    @MainActor
    func testStateMachineRestartOnUnexpectedExit() throws {
        // /usr/bin/false exits immediately — should trigger restarting state
        let binURL = URL(fileURLWithPath: "/usr/bin/false")
        let dataDir = URL(fileURLWithPath: NSTemporaryDirectory())
        let proc = WorkerProcess(workerBinaryURL: binURL, dataDir: dataDir)

        var states: [WorkerProcessState] = []
        let exp = expectation(description: "restarting state")
        var fulfilled = false
        proc.onStateChange = { state in
            states.append(state)
            if case .restarting = state, !fulfilled {
                fulfilled = true
                exp.fulfill()
            }
        }
        proc.start(env: [:])
        wait(for: [exp], timeout: 5)
        proc.stop()

        let hasRestarting = states.contains { if case .restarting = $0 { return true }; return false }
        XCTAssertTrue(hasRestarting, "expected .restarting state after unexpected exit, got \(states)")
    }
}
