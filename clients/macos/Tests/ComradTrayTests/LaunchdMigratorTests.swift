import XCTest
@testable import ComradTray

final class LaunchdMigratorTests: XCTestCase {

    func testMigrationNeededWhenFileExistsAndNotMigrated() {
        let tmpFile = NSTemporaryDirectory() + "com.comrad.worker.plist"
        FileManager.default.createFile(atPath: tmpFile, contents: nil)
        defer { try? FileManager.default.removeItem(atPath: tmpFile) }
        UserDefaults.standard.removeObject(forKey: "launchdMigrated")

        let migrator = LaunchdMigrator(plistPath: tmpFile)
        XCTAssertTrue(migrator.migrationNeeded())
    }

    func testMigrationNotNeededWhenFileAbsent() {
        UserDefaults.standard.removeObject(forKey: "launchdMigrated")
        let migrator = LaunchdMigrator(plistPath: "/nonexistent/path/com.comrad.worker.plist")
        XCTAssertFalse(migrator.migrationNeeded())
    }

    func testMigrationNotNeededWhenAlreadyMigrated() {
        let tmpFile = NSTemporaryDirectory() + "com.comrad.worker2.plist"
        FileManager.default.createFile(atPath: tmpFile, contents: nil)
        defer { try? FileManager.default.removeItem(atPath: tmpFile) }

        let migrator = LaunchdMigrator(plistPath: tmpFile)
        migrator.recordMigrated()
        defer { UserDefaults.standard.removeObject(forKey: "launchdMigrated") }

        XCTAssertFalse(migrator.migrationNeeded())
    }

    func testRecordMigratedSetsUserDefaults() {
        UserDefaults.standard.removeObject(forKey: "launchdMigrated")
        let migrator = LaunchdMigrator(plistPath: "/some/path")
        XCTAssertFalse(migrator.alreadyMigrated())
        migrator.recordMigrated()
        defer { UserDefaults.standard.removeObject(forKey: "launchdMigrated") }
        XCTAssertTrue(migrator.alreadyMigrated())
    }
}
