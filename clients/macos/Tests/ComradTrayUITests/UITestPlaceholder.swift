// UI regression tests require the built COMRAD.app bundle and XCUITest infrastructure.
// Run with: make uitest-tray-macos
// These tests are NOT part of the fast `make test-tray-macos` gate.
import XCTest

final class UITestPlaceholder: XCTestCase {
    func testPlaceholder() throws {
        throw XCTSkip("UI tests run via `make uitest-tray-macos` against the built .app")
    }
}
