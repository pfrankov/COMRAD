import Foundation

enum WorkerProcessState: Equatable {
    case idle
    case starting
    case running(pid: Int32)
    case stopping
    case stopped
    case restarting(attempt: Int)
    case failed(reason: String)
}

private let maxBackoffSeconds: Double = 30
private let backoffBase: Double = 1

func workerBackoff(attempt: Int) -> Double {
    min(backoffBase * pow(2.0, Double(attempt)), maxBackoffSeconds)
}

final class WorkerProcess {
    private(set) var state: WorkerProcessState = .idle
    var onStateChange: ((WorkerProcessState) -> Void)?

    private var process: Process?
    private var restartWorkItem: DispatchWorkItem?
    private var restartAttempt = 0
    private var lastEnv: [String: String] = [:]
    private let logFileURL: URL
    private let errLogFileURL: URL
    private let workerBinaryURL: URL
    private let pidFileURL: URL

    init(workerBinaryURL: URL, dataDir: URL) {
        self.workerBinaryURL = workerBinaryURL
        self.logFileURL = dataDir.appendingPathComponent("worker.log")
        self.errLogFileURL = dataDir.appendingPathComponent("worker.err.log")
        self.pidFileURL = dataDir.appendingPathComponent("worker.pid")
        killLeftoverWorker()
    }

    // Kills any worker processes left over from a previous session (crash / force-quit).
    // Uses the PID file first, then falls back to killing all processes with the same binary name.
    private func killLeftoverWorker() {
        var killedViaPID = false
        if let data = try? Data(contentsOf: pidFileURL),
           let pidString = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines),
           let pid = Int32(pidString), pid > 0 {
            if kill(pid, 0) == 0 {
                kill(pid, SIGTERM)
            }
            killedViaPID = true
        }
        try? FileManager.default.removeItem(at: pidFileURL)

        // Fallback: kill any leftover processes running the same binary (e.g. after a crash
        // that lost the PID file, or accumulated orphans from multiple restarts).
        let binaryPath = workerBinaryURL.path
        let task = Process()
        task.executableURL = URL(fileURLWithPath: "/usr/bin/pkill")
        task.arguments = ["-TERM", "-f", binaryPath]
        try? task.run()
        task.waitUntilExit()

        // Give processes a moment to exit before we launch a fresh one.
        if killedViaPID || (task.terminationStatus == 0) {
            Thread.sleep(forTimeInterval: 0.3)
        }
    }

    // Must be called on the main thread.
    func start(env: [String: String]) {
        guard case .idle = state else { return }
        restartAttempt = 0
        lastEnv = env
        launchWorker(env: env)
    }

    // Must be called on the main thread.
    func stop() {
        restartWorkItem?.cancel()
        restartWorkItem = nil
        let proc = process
        process = nil
        updateState(.stopping)
        if let proc, proc.isRunning {
            proc.terminationHandler = nil  // Prevent the async handler from treating this as a crash
            proc.terminate()
        }
        try? FileManager.default.removeItem(at: pidFileURL)
        updateState(.stopped)
    }

    // Must be called on the main thread.
    func restart(env: [String: String]) {
        stop()
        updateState(.idle)
        start(env: env)
    }

    private func launchWorker(env: [String: String]) {
        lastEnv = env
        let proc = Process()
        proc.executableURL = workerBinaryURL
        proc.environment = buildEnvironment(env: env)
        proc.standardOutput = logHandle()
        proc.standardError = errLogHandle()

        proc.terminationHandler = { [weak self] p in
            let exitCode = p.terminationStatus
            DispatchQueue.main.async {
                self?.handleTermination(exitCode: exitCode)
            }
        }

        updateState(.starting)
        do {
            try proc.run()
            process = proc
            let pid = proc.processIdentifier
            try? "\(pid)".write(to: pidFileURL, atomically: true, encoding: .utf8)
            updateState(.running(pid: pid))
        } catch {
            updateState(.failed(reason: error.localizedDescription))
        }
    }

    private func handleTermination(exitCode: Int32) {
        guard case .running = state else { return }
        restartAttempt += 1
        let attempt = restartAttempt
        updateState(.restarting(attempt: attempt))
        let delay = workerBackoff(attempt: attempt - 1)
        let env = lastEnv
        let item = DispatchWorkItem { [weak self] in
            guard let self else { return }
            if case .restarting = self.state {
                self.updateState(.idle)
                self.launchWorker(env: env)
            }
        }
        restartWorkItem = item
        DispatchQueue.main.asyncAfter(deadline: .now() + delay, execute: item)
    }

    private func updateState(_ newState: WorkerProcessState) {
        state = newState
        onStateChange?(newState)
    }

    private func buildEnvironment(env: [String: String]) -> [String: String] {
        var merged = ProcessInfo.processInfo.environment
        let binDir = workerBinaryURL.deletingLastPathComponent().path
        merged["DYLD_LIBRARY_PATH"] = binDir
        for (k, v) in env {
            merged[k] = v
        }
        return merged
    }

    private func logHandle() -> FileHandle? {
        FileManager.default.createFile(atPath: logFileURL.path, contents: nil)
        return try? FileHandle(forWritingTo: logFileURL)
    }

    private func errLogHandle() -> FileHandle? {
        FileManager.default.createFile(atPath: errLogFileURL.path, contents: nil)
        return try? FileHandle(forWritingTo: errLogFileURL)
    }

    static func resolveWorkerBinary() -> URL? {
        if let resourceURL = Bundle.main.resourceURL {
            let candidate = resourceURL.appendingPathComponent("bin/comrad-worker")
            if FileManager.default.fileExists(atPath: candidate.path) {
                return candidate
            }
        }
        let devCandidate = Bundle.main.bundleURL
            .deletingLastPathComponent()
            .appendingPathComponent("comrad-worker")
        if FileManager.default.fileExists(atPath: devCandidate.path) {
            return devCandidate
        }
        return nil
    }
}

extension WorkerProcessState: CustomStringConvertible {
    var description: String {
        switch self {
        case .idle: return "idle"
        case .starting: return "starting"
        case .running(let pid): return "running (pid \(pid))"
        case .stopping: return "stopping"
        case .stopped: return "stopped"
        case .restarting(let n): return "restarting (attempt \(n))"
        case .failed(let r): return "failed: \(r)"
        }
    }
}
