import AppKit
import CoreGraphics
import IOKit.ps

// IdleDetector determines whether the user is idle using HID input idle time.
//
// Idle = no keyboard/mouse/trackpad input for ≥ 2 minutes AND the machine is
// not in a throttled state (battery + display asleep).
//
// Why input idle time and not display sleep:
//   - Display sleep is a lagging indicator: the user may have walked away 4
//     minutes ago while the display timeout is still counting down.
//   - On Apple Silicon on battery, display sleep triggers aggressive CPU/GPU
//     clock reduction. Running inference in that state produces slow results
//     and risks timeouts; better to stay paused until the machine is on AC.
//   - Apps that call caffeinate or prevent display sleep (Zoom, presentations)
//     keep the display on even when the user has left the room.

final class IdleDetector {
    enum UserState: Equatable { case idle, active }

    static let idleThresholdSeconds: Double = 120

    // Injected for testing.
    var inputIdleSeconds: () -> Double = {
        // kCGAnyInputEventType isn't directly usable as a Swift CGEventType case,
        // so we take the minimum across event types that cover all HID input.
        let types: [CGEventType] = [
            .keyDown, .mouseMoved, .leftMouseDown, .rightMouseDown,
            .otherMouseDown, .scrollWheel,
        ]
        return types
            .map { CGEventSource.secondsSinceLastEventType(.hidSystemState, eventType: $0) }
            .min() ?? .infinity
    }
    var isDisplayAsleep: () -> Bool = {
        CGDisplayIsAsleep(CGMainDisplayID()) != 0
    }
    var isOnBattery: () -> Bool = {
        guard let infoRef = IOPSCopyPowerSourcesInfo() else { return false }
        let info = infoRef.takeRetainedValue()
        guard let listRef = IOPSCopyPowerSourcesList(info) else { return false }
        let list = listRef.takeRetainedValue() as [AnyObject]
        for item in list {
            guard let descRef = IOPSGetPowerSourceDescription(info, item as CFTypeRef),
                  let desc = descRef.takeUnretainedValue() as? [String: AnyObject],
                  let state = desc[kIOPSPowerSourceStateKey] as? String
            else { continue }
            if state == kIOPSACPowerValue { return false }
        }
        return true
    }

    private(set) var state: UserState = .active
    var onStateChange: ((UserState) -> Void)?

    private var timer: Timer?

    // Must be called on the main thread.
    func start() {
        check()
        timer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { [weak self] _ in
            self?.check()
        }
    }

    // Must be called on the main thread.
    func stop() {
        timer?.invalidate()
        timer = nil
    }

    func check() {
        let inputIdle = inputIdleSeconds() >= Self.idleThresholdSeconds
        // Don't run inference when battery + display off: machine is throttled.
        let throttled = isOnBattery() && isDisplayAsleep()
        let newState: UserState = (inputIdle && !throttled) ? .idle : .active
        guard newState != state else { return }
        state = newState
        onStateChange?(newState)
    }
}
