import Foundation

// WorkerControl calls the worker's local loopback-only control endpoints.
// No authentication is required — these endpoints only bind to 127.0.0.1.

final class WorkerControl {
    private let session: URLSession

    init(session: URLSession = .shared) {
        self.session = session
    }

    func setPaused(_ paused: Bool, port: Int, completion: ((Bool) -> Void)? = nil) {
        let path = paused ? "/pause" : "/resume"
        guard let url = URL(string: "http://127.0.0.1:\(port)\(path)") else {
            completion?(false); return
        }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 5)
        req.httpMethod = "POST"
        session.dataTask(with: req) { _, resp, _ in
            let ok = (resp as? HTTPURLResponse)?.statusCode == 200
            DispatchQueue.main.async { completion?(ok) }
        }.resume()
    }
}
