import XCTest
@testable import ComradTray

// MockURLProtocol captures requests and returns a canned response.
class MockURLProtocol: URLProtocol {
    static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let handler = Self.requestHandler else {
            client?.urlProtocol(self, didFailWithError: URLError(.unknown))
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

private func makeMockSession() -> URLSession {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [MockURLProtocol.self]
    return URLSession(configuration: config)
}

private func okResponse(for url: URL) -> HTTPURLResponse {
    HTTPURLResponse(url: url, statusCode: 200, httpVersion: nil, headerFields: nil)!
}

final class WorkerControlTests: XCTestCase {

    override func setUp() {
        super.setUp()
        MockURLProtocol.requestHandler = nil
    }

    func testPausePostsToPauseEndpoint() {
        let session = makeMockSession()
        let control = WorkerControl(session: session)

        var captured: URLRequest?
        MockURLProtocol.requestHandler = { req in
            captured = req
            return (okResponse(for: req.url!), Data())
        }

        let exp = expectation(description: "completion")
        control.setPaused(true, port: 1923) { _ in exp.fulfill() }
        wait(for: [exp], timeout: 3)

        XCTAssertEqual(captured?.url?.path, "/pause")
        XCTAssertEqual(captured?.httpMethod, "POST")
        XCTAssertEqual(captured?.url?.port, 1923)
    }

    func testResumePostsToResumeEndpoint() {
        let session = makeMockSession()
        let control = WorkerControl(session: session)

        var captured: URLRequest?
        MockURLProtocol.requestHandler = { req in
            captured = req
            return (okResponse(for: req.url!), Data())
        }

        let exp = expectation(description: "completion")
        control.setPaused(false, port: 1923) { _ in exp.fulfill() }
        wait(for: [exp], timeout: 3)

        XCTAssertEqual(captured?.url?.path, "/resume")
        XCTAssertEqual(captured?.httpMethod, "POST")
    }

    func testCallbackReceivesTrueOnHTTP200() {
        let session = makeMockSession()
        let control = WorkerControl(session: session)

        MockURLProtocol.requestHandler = { req in (okResponse(for: req.url!), Data()) }

        var result: Bool?
        let exp = expectation(description: "done")
        control.setPaused(true, port: 1923) { ok in
            result = ok; exp.fulfill()
        }
        wait(for: [exp], timeout: 3)
        XCTAssertEqual(result, true)
    }

    func testCallbackReceivesFalseOnHTTP500() {
        let session = makeMockSession()
        let control = WorkerControl(session: session)

        MockURLProtocol.requestHandler = { req in
            let resp = HTTPURLResponse(url: req.url!, statusCode: 500, httpVersion: nil, headerFields: nil)!
            return (resp, Data())
        }

        var result: Bool?
        let exp = expectation(description: "done")
        control.setPaused(true, port: 1923) { ok in
            result = ok; exp.fulfill()
        }
        wait(for: [exp], timeout: 3)
        XCTAssertEqual(result, false)
    }

    func testUsesLoopbackAddress() {
        let session = makeMockSession()
        let control = WorkerControl(session: session)

        var captured: URLRequest?
        MockURLProtocol.requestHandler = { req in
            captured = req
            return (okResponse(for: req.url!), Data())
        }

        let exp = expectation(description: "done")
        control.setPaused(true, port: 9999) { _ in exp.fulfill() }
        wait(for: [exp], timeout: 3)

        XCTAssertEqual(captured?.url?.host, "127.0.0.1")
        XCTAssertEqual(captured?.url?.port, 9999)
    }
}
