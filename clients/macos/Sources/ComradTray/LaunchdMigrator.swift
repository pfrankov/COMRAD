import Foundation
import AppKit

private let migratedKey = "launchdMigrated"
private let defaultPlistPath = "Library/LaunchAgents/com.comrad.worker.plist"

struct LaunchdMigrator {
    let plistPath: String

    init(plistPath: String? = nil) {
        if let path = plistPath {
            self.plistPath = path
        } else {
            let home = FileManager.default.homeDirectoryForCurrentUser.path
            self.plistPath = "\(home)/\(defaultPlistPath)"
        }
    }

    func migrationNeeded() -> Bool {
        guard !alreadyMigrated() else { return false }
        return FileManager.default.fileExists(atPath: plistPath)
    }

    func alreadyMigrated() -> Bool {
        UserDefaults.standard.bool(forKey: migratedKey)
    }

    func recordMigrated() {
        UserDefaults.standard.set(true, forKey: migratedKey)
    }

    func performMigration() {
        let proc = Process()
        proc.executableURL = URL(fileURLWithPath: "/bin/launchctl")
        proc.arguments = ["unload", plistPath]
        try? proc.run()
        proc.waitUntilExit()
        recordMigrated()
    }

    func offerToRemovePlist() {
        let alert = NSAlert()
        alert.messageText = "Remove launchd job file?"
        alert.informativeText = "COMRAD is now managed by the menu-bar app.\nThe old launchd plist at:\n\(plistPath)\ncan be removed."
        alert.addButton(withTitle: "Remove")
        alert.addButton(withTitle: "Keep")
        if alert.runModal() == .alertFirstButtonReturn {
            try? FileManager.default.removeItem(atPath: plistPath)
        }
    }
}
