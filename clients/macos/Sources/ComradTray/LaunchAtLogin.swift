import Foundation
import ServiceManagement

enum LaunchAtLoginError: Error {
    case unavailable
}

struct LaunchAtLogin {
    static func register() throws {
        if #available(macOS 13.0, *) {
            try SMAppService.mainApp.register()
        } else {
            throw LaunchAtLoginError.unavailable
        }
    }

    static func unregister() throws {
        if #available(macOS 13.0, *) {
            try SMAppService.mainApp.unregister()
        } else {
            throw LaunchAtLoginError.unavailable
        }
    }

    static var isRegistered: Bool {
        if #available(macOS 13.0, *) {
            return SMAppService.mainApp.status == .enabled
        }
        return false
    }
}
