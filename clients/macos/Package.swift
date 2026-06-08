// swift-tools-version: 5.9
import PackageDescription

// Note: ComradTrayUITests (XCUITest regression tests) require an Xcode project and
// must be run via `make uitest-tray-macos` against the built COMRAD.app.
// They are not included here because XCUITest is not available in pure SwiftPM.
let package = Package(
    name: "ComradTray",
    platforms: [.macOS(.v13)],
    targets: [
        .executableTarget(
            name: "ComradTray",
            path: "Sources/ComradTray"
        ),
        .testTarget(
            name: "ComradTrayTests",
            dependencies: ["ComradTray"],
            path: "Tests/ComradTrayTests"
        ),
    ]
)
