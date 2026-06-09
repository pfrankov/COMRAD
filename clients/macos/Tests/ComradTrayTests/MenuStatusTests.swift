import XCTest
@testable import ComradTray

// MARK: - Fixtures

private func snap(
    connected: Bool = true,
    slots: [WorkerSlotStatus] = [],
    warmCount: Int = 0,
    paused: Bool? = nil,
    lastError: String? = nil,
    managerUrl: String = "http://manager.example.com",
    p2p: WorkerP2PStatus? = nil
) -> WorkerStatusSnapshot {
    WorkerStatusSnapshot(
        connected: connected,
        nodeId: "n1",
        nodeName: "Test Node",
        version: "1.0.0",
        target: "test",
        runtimeAdapters: [],
        slots: slots,
        cachedCount: 0,
        warmCount: warmCount,
        warmProfiles: [],
        p2p: p2p,
        managerUrl: managerUrl,
        lastError: lastError,
        paused: paused,
        startedAt: "2024-01-01T00:00:00Z",
        updatedAt: "2024-01-01T00:00:00Z"
    )
}

private func slots(_ states: String...) -> [WorkerSlotStatus] {
    states.enumerated().map { i, s in WorkerSlotStatus(id: "s\(i)", state: s) }
}

// MARK: - Tests

final class MenuStatusTests: XCTestCase {

    // MARK: Non-connected states

    func testStopped() {
        let line = makeStatusLine(
            pollerState: .stopped,
            workerState: .stopped,
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Stopped")
        XCTAssertNil(line.secondary)
        XCTAssertEqual(line.tone, .idle)
    }

    func testConnecting() {
        let line = makeStatusLine(
            pollerState: .connecting,
            workerState: .starting,
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertTrue(line.primary.hasPrefix("Starting up"))
        XCTAssertNil(line.secondary)
        XCTAssertEqual(line.tone, .idle)
    }

    func testErrorWorkerNotResponding() {
        let line = makeStatusLine(
            pollerState: .error("connection refused"),
            workerState: .running(pid: 0),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Not responding")
        XCTAssertNil(line.secondary)
        XCTAssertEqual(line.tone, .error)
    }

    func testErrorWithRestartAttempt() {
        let line = makeStatusLine(
            pollerState: .error("i/o timeout"),
            workerState: .restarting(attempt: 3),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Not responding")
        XCTAssertEqual(line.secondary, "Restarting \u{2014} attempt 3")
        XCTAssertEqual(line.tone, .error)
    }

    // MARK: Idle-only mode

    func testIdleOnlyModeUserActive() {
        let line = makeStatusLine(
            pollerState: .connected(snap()),
            workerState: .running(pid: 1),
            idleOnlyMode: true,
            userActive: true
        )
        XCTAssertEqual(line.primary, "Paused")
        XCTAssertTrue(line.secondary?.contains("Mac in use") == true)
        XCTAssertTrue(line.secondary?.contains("resumes when idle") == true)
        XCTAssertEqual(line.tone, .paused)
    }

    func testIdleOnlyModeUserIdle() {
        // When user is idle, idle-only doesn't suppress normal status
        let line = makeStatusLine(
            pollerState: .connected(snap()),
            workerState: .running(pid: 1),
            idleOnlyMode: true,
            userActive: false
        )
        XCTAssertEqual(line.tone, .ready)
    }

    // MARK: Manager disconnected (warning, not error)

    func testDisconnectedFromManagerDefaultMessage() {
        let line = makeStatusLine(
            pollerState: .connected(snap(connected: false, managerUrl: "")),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Disconnected from manager")
        XCTAssertNil(line.secondary)  // no URL → no secondary
        XCTAssertEqual(line.tone, .warning)  // yellow, not red
    }

    func testDisconnectedManagerHostInSecondary() {
        let line = makeStatusLine(
            pollerState: .connected(snap(connected: false, managerUrl: "http://mymanager.local:8080")),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertNil(line.secondary)
        XCTAssertEqual(line.tone, .warning)
    }

    func testDisconnectedConnectionRefused() {
        let line = makeStatusLine(
            pollerState: .connected(snap(connected: false, lastError: "connection refused")),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Manager is not running")
        XCTAssertEqual(line.tone, .warning)
    }

    func testDisconnectedTimeout() {
        let line = makeStatusLine(
            pollerState: .connected(snap(connected: false, lastError: "i/o timeout")),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Manager is unreachable")
        XCTAssertEqual(line.tone, .warning)
    }

    // MARK: Paused

    func testPausedManually() {
        let line = makeStatusLine(
            pollerState: .connected(snap(paused: true)),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Paused")
        XCTAssertNil(line.secondary)
        XCTAssertEqual(line.tone, .paused)
    }

    func testPausedWhileServingFinishesActiveRequests() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("serving", "serving"), paused: true)),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Paused")
        XCTAssertTrue(line.secondary?.contains("2 active requests") == true)
        XCTAssertEqual(line.tone, .paused)
    }

    // MARK: Working

    func testWorkingOneOfTwo() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("serving", "ready"))),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Working \u{2014} 1 of 2")
        XCTAssertEqual(line.tone, .working)
    }

    func testWorkingNoSecondary() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("serving", "ready"), warmCount: 3)),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertNil(line.secondary)
    }

    // MARK: Loading

    func testLoadingWarming() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("warming", "ready"))),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertTrue(line.primary.hasPrefix("Loading model"))
        XCTAssertEqual(line.tone, .loading)
    }

    func testLoadingDownloading() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("downloading_artifact", "idle"))),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.tone, .loading)
    }

    func testLoadingLoadingRuntime() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("loading_runtime"))),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.tone, .loading)
    }

    func testLoadingNoSecondary() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("warming", "warming", "ready", "ready"))),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertNil(line.secondary)
    }

    // MARK: Ready

    func testReadyNoModels() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("ready", "ready"))),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertEqual(line.primary, "Ready")
        XCTAssertNil(line.secondary)
        XCTAssertEqual(line.tone, .ready)
    }

    func testReadyWithWarmModels() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("ready"), warmCount: 2)),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertTrue(line.secondary?.contains("2 models warm") == true)
    }

    func testReadyNoWarmModelsNoSecondary() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("ready"))),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertNil(line.secondary)
    }

    func testReadySingularModel() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("ready"), warmCount: 1)),
            workerState: .running(pid: 1),
            idleOnlyMode: false,
            userActive: false
        )
        XCTAssertTrue(line.secondary?.contains("1 model warm") == true)
        XCTAssertFalse(line.secondary?.contains("1 models") == true)
    }

    // MARK: Pulse behaviour

    func testPulseOnWorking() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("serving", "ready"))),
            workerState: .running(pid: 1), idleOnlyMode: false, userActive: false
        )
        XCTAssertTrue(line.shouldPulse)
    }

    func testPulseOnLoading() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("warming"))),
            workerState: .running(pid: 1), idleOnlyMode: false, userActive: false
        )
        XCTAssertTrue(line.shouldPulse)
    }

    func testPulseOnConnecting() {
        let line = makeStatusLine(
            pollerState: .connecting,
            workerState: .starting, idleOnlyMode: false, userActive: false
        )
        XCTAssertTrue(line.shouldPulse)
    }

    func testPulseOnRestarting() {
        let line = makeStatusLine(
            pollerState: .error("timeout"),
            workerState: .restarting(attempt: 1), idleOnlyMode: false, userActive: false
        )
        XCTAssertTrue(line.shouldPulse)
    }

    func testNoPulseOnReady() {
        let line = makeStatusLine(
            pollerState: .connected(snap(slots: slots("ready"))),
            workerState: .running(pid: 1), idleOnlyMode: false, userActive: false
        )
        XCTAssertFalse(line.shouldPulse)
    }

    func testNoPulseOnStopped() {
        let line = makeStatusLine(
            pollerState: .stopped,
            workerState: .stopped, idleOnlyMode: false, userActive: false
        )
        XCTAssertFalse(line.shouldPulse)
    }

    func testNoPulseOnError() {
        let line = makeStatusLine(
            pollerState: .error("refused"),
            workerState: .running(pid: 0), idleOnlyMode: false, userActive: false
        )
        XCTAssertFalse(line.shouldPulse)
    }

    func testNoPulseOnPaused() {
        let line = makeStatusLine(
            pollerState: .connected(snap(paused: true)),
            workerState: .running(pid: 1), idleOnlyMode: false, userActive: false
        )
        XCTAssertFalse(line.shouldPulse)
    }

    // MARK: Slot helpers

    func testServingCountUsesTrueServingState() {
        let s = slots("serving", "serving", "ready", "idle")
        XCTAssertEqual(s.servingCount, 2)
        // The old code used "busy" — ensure "busy" is NOT counted
        let old = slots("busy", "busy", "ready")
        XCTAssertEqual(old.servingCount, 0)
    }

    func testLoadingCountCoversAllTransitionalStates() {
        let s = slots("warming", "loading_runtime", "downloading_artifact", "download_queued",
                      "ready", "serving", "idle", "cached")
        XCTAssertEqual(s.loadingCount, 4) // cached is steady-state, not loading
    }
}
