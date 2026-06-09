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
        let loc = Localization.shared
        let alert = NSAlert()
        alert.messageText = loc.t("alert.removePlistTitle")
        alert.informativeText = loc.t("alert.removePlistDetail", values: ["path": plistPath])
        alert.addButton(withTitle: loc.t("alert.remove"))
        alert.addButton(withTitle: loc.t("alert.keep"))
        if alert.runModal() == .alertFirstButtonReturn {
            try? FileManager.default.removeItem(atPath: plistPath)
        }
    }
}
