import Foundation

// WorkerControl calls the worker's local loopback-only control endpoints.
// No authentication is required — these endpoints only bind to 127.0.0.1.

struct CachedArtifactInfo: Decodable {
    var id: String
    var sizeBytes: Int64
    var profiles: [String]?
}

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

    func fetchCache(port: Int, completion: @escaping ([CachedArtifactInfo]) -> Void) {
        guard let url = URL(string: "http://127.0.0.1:\(port)/cache") else {
            completion([]); return
        }
        let req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 5)
        session.dataTask(with: req) { data, resp, _ in
            let items: [CachedArtifactInfo]
            if let data, (resp as? HTTPURLResponse)?.statusCode == 200,
               let decoded = try? JSONDecoder().decode([CachedArtifactInfo].self, from: data) {
                items = decoded
            } else {
                items = []
            }
            DispatchQueue.main.async { completion(items) }
        }.resume()
    }

    func evictCachedArtifact(port: Int, artifactId: String, completion: ((Bool) -> Void)? = nil) {
        let encoded = artifactId.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? artifactId
        guard let url = URL(string: "http://127.0.0.1:\(port)/cache?artifactId=\(encoded)") else {
            completion?(false); return
        }
        var req = URLRequest(url: url, cachePolicy: .reloadIgnoringLocalCacheData, timeoutInterval: 10)
        req.httpMethod = "DELETE"
        session.dataTask(with: req) { _, resp, _ in
            let ok = (resp as? HTTPURLResponse)?.statusCode == 200
            DispatchQueue.main.async { completion?(ok) }
        }.resume()
    }
}
