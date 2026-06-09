import AppKit

// MARK: - Tone

enum StatusTone: Equatable {
    case ready, working, loading, paused, warning, error, idle

    var color: NSColor {
        switch self {
        case .ready:   return .systemGreen
        case .working: return .systemBlue
        case .loading: return .systemTeal
        case .paused:  return .systemOrange
        case .warning: return .systemYellow
        case .error:   return .systemRed
        case .idle:    return .tertiaryLabelColor
        }
    }
}

// MARK: - StatusLine

struct StatusLine: Equatable {
    let primary: String
    let secondary: String?
    let tone: StatusTone
    let shouldPulse: Bool
}

// MARK: - Slot helpers

extension Array where Element == WorkerSlotStatus {
    /// Slots actively serving a request.
    var servingCount: Int { filter { $0.state == "serving" }.count }

    /// Slots in a transitional loading/warming state (not yet ready, not serving).
    var loadingCount: Int {
        filter {
            switch $0.state {
            case "warming", "loading_runtime", "downloading_artifact", "download_queued":
                return true
            default:
                return false
            }
        }.count
    }
}

// MARK: - Pure presentation function

/// Returns a two-line status description driven purely by current state. No AppKit
/// dependencies — safe to unit-test without a running menu.
func makeStatusLine(
    pollerState: PollerState,
    workerState: WorkerProcessState,
    idleOnlyMode: Bool,
    userActive: Bool
) -> StatusLine {
    let loc = Localization.shared

    // 1. Idle-only mode: user is at the keyboard → worker is intentionally paused.
    if idleOnlyMode && userActive {
        return StatusLine(
            primary: loc.t("status.paused"),
            secondary: loc.t("status.macInUse"),
            tone: .paused,
            shouldPulse: false
        )
    }

    switch pollerState {

    // 2. Worker process is not responding at all.
    case .error:
        let isRestarting: Bool
        let hint: String?
        if case .restarting(let attempt) = workerState {
            hint = loc.t("status.restarting", values: ["attempt": attempt])
            isRestarting = true
        } else {
            hint = nil
            isRestarting = false
        }
        return StatusLine(primary: loc.t("status.notResponding"), secondary: hint, tone: .error, shouldPulse: isRestarting)

    // 3. Worker is stopped (user pressed Stop).
    case .stopped:
        return StatusLine(primary: loc.t("status.stopped"), secondary: nil, tone: .idle, shouldPulse: false)

    // 4. Waiting for the local status endpoint to come up.
    case .connecting:
        return StatusLine(primary: loc.t("status.startingUp"), secondary: nil, tone: .idle, shouldPulse: true)

    // 5. HTTP /status responded — inspect the snapshot.
    case .connected(let snap):

        // 5a. Worker is alive locally but hasn't registered with the manager.
        guard snap.connected else {
            return StatusLine(primary: humanizeConnectionError(snap.lastError), secondary: nil, tone: .warning, shouldPulse: false)
        }

        let slots = snap.slots
        let total = slots.count
        let serving = slots.servingCount
        let loading = slots.loadingCount

        // 5b. Paused via /pause endpoint (manual or externally triggered).
        if snap.paused == true {
            let detail: String? = serving > 0
                ? loc.t("status.finishingRequests_\(serving == 1 ? "one" : "other")", values: ["count": serving])
                : nil
            return StatusLine(primary: loc.t("status.paused"), secondary: detail, tone: .paused, shouldPulse: false)
        }

        // 5c. Actively serving at least one request.
        if serving > 0 {
            return StatusLine(
                primary: loc.t("status.working", values: ["serving": serving, "total": total]),
                secondary: nil,
                tone: .working,
                shouldPulse: true
            )
        }

        // 5d. Models are downloading / loading runtime / warming up.
        if loading > 0 {
            return StatusLine(
                primary: loc.t("status.loadingModel"),
                secondary: nil,
                tone: .loading,
                shouldPulse: true
            )
        }

        // 5e. Idle and ready to accept work.
        let warmModelsSecondary: String? = snap.warmCount > 0
            ? loc.t("status.modelsWarm_\(snap.warmCount == 1 ? "one" : "other")", values: ["count": snap.warmCount])
            : nil
        return StatusLine(
            primary: loc.t("status.ready"),
            secondary: warmModelsSecondary,
            tone: .ready,
            shouldPulse: false
        )
    }
}

// MARK: - Connection error humanizer

func humanizeConnectionError(_ error: String?) -> String {
    let loc = Localization.shared
    guard let error else { return loc.t("status.disconnected") }
    if error.contains("i/o timeout") || error.contains("timed out") {
        return loc.t("status.unreachable")
    }
    if error.contains("connection refused") {
        return loc.t("status.notRunning")
    }
    if error.contains("no such host") || error.contains("no route to host") {
        return loc.t("status.hostNotFound")
    }
    return loc.t("status.disconnected")
}

// MARK: - Visibility helper (used by MenuController for tray icon alpha)

func isConnectedToManager(_ pollerState: PollerState) -> Bool {
    if case .connected(let snap) = pollerState { return snap.connected }
    return false
}

