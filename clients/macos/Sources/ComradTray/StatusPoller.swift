import Foundation

// WorkerStatusSnapshot mirrors the Go JSON contract in worker_status.go.
// Field names are the contract — do not rename without coordinating with Go.
struct WorkerStatusSnapshot: Decodable {
    var connected: Bool
    var nodeId: String
    var nodeName: String
    var version: String
    var target: String
    var runtimeAdapters: [String]
    var slots: [WorkerSlotStatus]
    var cachedCount: Int
    var warmCount: Int
    var warmProfiles: [String]
    var p2p: WorkerP2PStatus?
    var managerUrl: String
    var lastError: String?
    var paused: Bool?
    var startedAt: String
    var updatedAt: String
}

struct WorkerSlotStatus: Decodable {
    var id: String
    var state: String
}

struct WorkerP2PStatus: Decodable {
    var available: Bool
    var port: Int?
    var peers: Int?
    var seedingCount: Int?
    var downloadingCount: Int?
    var fallbackCount: Int?
    var lastFailure: String?
}

enum PollerState {
    case connecting
    case connected(WorkerStatusSnapshot)
    case error(String)
    case stopped
}

final class StatusPoller {
    private(set) var state: PollerState = .connecting
    var onStateChange: ((PollerState) -> Void)?

    private var timer: Timer?
    private let session: URLSession
    private var port: Int

    init(port: Int, session: URLSession = .shared) {
        self.port = port
        self.session = session
    }

    // Must be called on the main thread.
    func start() {
        poll()
        timer = Timer.scheduledTimer(withTimeInterval: 2.0, repeats: true) { [weak self] _ in
            self?.poll()
        }
    }

    // Must be called on the main thread.
    func stop() {
        timer?.invalidate()
        timer = nil
        updateState(.stopped)
    }

    func updatePort(_ newPort: Int) {
        port = newPort
    }

    private func poll() {
        let currentPort = port
        guard let url = URL(string: "http://127.0.0.1:\(currentPort)/status") else { return }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 2)
        req.timeoutInterval = 2
        let task = session.dataTask(with: req) { [weak self] data, response, error in
            DispatchQueue.main.async {
                guard let self else { return }
                if let error {
                    self.updateState(.error(error.localizedDescription))
                    return
                }
                guard let http = response as? HTTPURLResponse, http.statusCode == 200,
                      let data else {
                    let code = (response as? HTTPURLResponse)?.statusCode ?? 0
                    self.updateState(.error("HTTP \(code)"))
                    return
                }
                do {
                    let snap = try JSONDecoder().decode(WorkerStatusSnapshot.self, from: data)
                    self.updateState(.connected(snap))
                } catch {
                    self.updateState(.error(error.localizedDescription))
                }
            }
        }
        task.resume()
    }

    private func updateState(_ newState: PollerState) {
        state = newState
        onStateChange?(newState)
    }
}
