import XCTest
@testable import ComradTray

final class IdleDetectorTests: XCTestCase {

    private func makeDetector(
        idleSeconds: Double = 0,
        displayAsleep: Bool = false,
        onBattery: Bool = false
    ) -> IdleDetector {
        let d = IdleDetector()
        d.inputIdleSeconds = { idleSeconds }
        d.isDisplayAsleep  = { displayAsleep }
        d.isOnBattery      = { onBattery }
        return d
    }

    // MARK: Basic cases

    func testStartsActive() {
        let d = makeDetector()
        XCTAssertEqual(d.state, .active)
    }

    func testRecentInputIsActive() {
        let d = makeDetector(idleSeconds: 30)
        d.check()
        XCTAssertEqual(d.state, .active)
    }

    func testInputIdleAboveThresholdOnACIsIdle() {
        let d = makeDetector(idleSeconds: 121, onBattery: false)
        d.check()
        XCTAssertEqual(d.state, .idle)
    }

    func testInputIdleExactlyAtThresholdIsIdle() {
        let d = makeDetector(idleSeconds: IdleDetector.idleThresholdSeconds, onBattery: false)
        d.check()
        XCTAssertEqual(d.state, .idle)
    }

    // MARK: Battery + display sleep throttle guard

    func testBatteryPlusDisplaySleepStaysActiveEvenIfInputIdle() {
        // Machine is throttled — don't run inference even though user is away.
        let d = makeDetector(idleSeconds: 300, displayAsleep: true, onBattery: true)
        d.check()
        XCTAssertEqual(d.state, .active, "battery + display sleep = throttled, must stay active")
    }

    func testBatteryDisplayOnAndIdleIsIdle() {
        // On battery but caffeinate/Zoom is keeping the display awake — not throttled.
        let d = makeDetector(idleSeconds: 300, displayAsleep: false, onBattery: true)
        d.check()
        XCTAssertEqual(d.state, .idle)
    }

    func testACDisplaySleepAndIdleIsIdle() {
        // Plugged in, display slept — not throttled, inference is fine.
        let d = makeDetector(idleSeconds: 300, displayAsleep: true, onBattery: false)
        d.check()
        XCTAssertEqual(d.state, .idle)
    }

    // MARK: Transitions and callbacks

    func testCallbackFiredOnTransitionToIdle() {
        var seconds = 0.0
        let d = IdleDetector()
        d.inputIdleSeconds = { seconds }
        d.isDisplayAsleep  = { false }
        d.isOnBattery      = { false }

        var received: [IdleDetector.UserState] = []
        d.onStateChange = { received.append($0) }

        d.check()           // 0 s idle → active, no callback
        seconds = 200
        d.check()           // 200 s idle → idle, callback fires
        XCTAssertEqual(received, [.idle])
    }

    func testCallbackFiredOnTransitionToActive() {
        var seconds = 200.0
        let d = IdleDetector()
        d.inputIdleSeconds = { seconds }
        d.isDisplayAsleep  = { false }
        d.isOnBattery      = { false }

        d.check()
        XCTAssertEqual(d.state, .idle)

        var received: [IdleDetector.UserState] = []
        d.onStateChange = { received.append($0) }

        seconds = 5
        d.check()           // idle → active (user came back)
        XCTAssertEqual(received, [.active])
    }

    func testNoCallbackWhenStateUnchanged() {
        let d = makeDetector(idleSeconds: 200)
        d.check()           // → idle (no callback, onStateChange not wired yet)

        var callCount = 0
        d.onStateChange = { _ in callCount += 1 }
        d.check()
        d.check()
        XCTAssertEqual(callCount, 0)
    }

    func testTransitionsFromIdleToThrottledStayActive() {
        // User was idle on AC, then unplugged — machine enters throttled state.
        var onBattery = false
        var displayAsleep = false
        let d = IdleDetector()
        d.inputIdleSeconds = { 300 }
        d.isDisplayAsleep  = { displayAsleep }
        d.isOnBattery      = { onBattery }

        d.check()
        XCTAssertEqual(d.state, .idle)  // AC, display on, idle → idle

        onBattery = true
        displayAsleep = true
        d.check()
        XCTAssertEqual(d.state, .active, "should pause when battery + display sleep")
    }
}
