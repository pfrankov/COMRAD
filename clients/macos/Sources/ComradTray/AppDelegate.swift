import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    private var statusItem: NSStatusItem!
    private var menuController: MenuController!
    private var workerProcess: WorkerProcess?
    private var statusPoller: StatusPoller!
    private var settingsWindowController: SettingsWindowController?
    private let store = SettingsStore()
    private var settings: Settings = Settings()

    private let idleDetector = IdleDetector()
    private let workerControl = WorkerControl()

    func applicationDidFinishLaunching(_ notification: Notification) {
        settings = store.load()

        if settings.language.isEmpty {
            settings.language = Localization.systemLanguage()
            try? store.save(settings)
        }
        Localization.shared = Localization(language: settings.language)

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)

        menuController = MenuController(statusItem: statusItem)
        wireMenuActions()

        statusPoller = StatusPoller(port: settings.statusPort)
        statusPoller.onStateChange = { [weak self] state in
            DispatchQueue.main.async { self?.pollerDidUpdate(state) }
        }

        idleDetector.onStateChange = { [weak self] _ in
            self?.idleStateDidChange()
        }

        migrateIfNeeded()
        startWorkerIfNeeded()
        statusPoller.start()

        if settings.idleOnlyMode {
            idleDetector.start()
        }

        refreshMenu()
    }

    func applicationWillTerminate(_ notification: Notification) {
        workerProcess?.stop()
    }

    // MARK: Private

    private func wireMenuActions() {
        menuController.onStartWorker = { [weak self] in self?.startWorkerIfNeeded() }
        menuController.onStopWorker = { [weak self] in self?.workerProcess?.stop() }
        menuController.onOpenSettings = { [weak self] in self?.openSettings() }
        menuController.onOpenLogs = { [weak self] in self?.openLogs() }
        menuController.onToggleLaunchAtLogin = { [weak self] in self?.toggleLaunchAtLogin() }
        menuController.onToggleIdleOnlyMode = { [weak self] in self?.toggleIdleOnlyMode() }
        menuController.onQuit = { NSApp.terminate(nil) }
    }

    private func migrateIfNeeded() {
        let migrator = LaunchdMigrator()
        if migrator.migrationNeeded() {
            migrator.performMigration()
            migrator.offerToRemovePlist()
        }
    }

    private func startWorkerIfNeeded() {
        guard workerProcess == nil else {
            workerProcess?.restart(env: settings.envVars())
            return
        }
        guard let binaryURL = WorkerProcess.resolveWorkerBinary() else {
            let loc = Localization.shared
            let alert = NSAlert()
            alert.messageText = loc.t("alert.binaryNotFound")
            alert.informativeText = loc.t("alert.binaryNotFoundDetail")
            alert.runModal()
            return
        }
        let dataDir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)
            .first!.appendingPathComponent("COMRAD")
        let proc = WorkerProcess(workerBinaryURL: binaryURL, dataDir: dataDir)
        proc.onStateChange = { [weak self] state in
            DispatchQueue.main.async { self?.workerDidChangeState(state) }
        }
        workerProcess = proc
        proc.start(env: settings.envVars())
    }

    private func openSettings() {
        settingsWindowController = SettingsWindowController(store: store) { [weak self] newSettings in
            guard let self else { return }
            let oldLanguage = self.settings.language
            let changed = newSettings != self.settings
            self.settings = newSettings
            self.statusPoller.updatePort(newSettings.statusPort)
            if changed {
                if newSettings.language != oldLanguage {
                    Localization.shared = Localization(language: newSettings.language)
                }
                self.workerProcess?.restart(env: self.settings.envVars())
            }
        }
        settingsWindowController?.showWindow(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    private func openLogs() {
        let dataDir = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)
            .first!.appendingPathComponent("COMRAD")
        let logURL = dataDir.appendingPathComponent("worker.log")
        if FileManager.default.fileExists(atPath: logURL.path) {
            NSWorkspace.shared.open(logURL)
        } else {
            NSWorkspace.shared.open(dataDir)
        }
    }

    private func toggleLaunchAtLogin() {
        if LaunchAtLogin.isRegistered {
            try? LaunchAtLogin.unregister()
        } else {
            try? LaunchAtLogin.register()
        }
        settings.launchAtLogin = LaunchAtLogin.isRegistered
        try? store.save(settings)
        refreshMenu()
    }

    private func toggleIdleOnlyMode() {
        settings.idleOnlyMode.toggle()
        try? store.save(settings)

        if settings.idleOnlyMode {
            idleDetector.start()
            applyIdleState()
        } else {
            idleDetector.stop()
            // Resume worker since idle-only is off
            workerControl.setPaused(false, port: settings.statusPort)
        }
        refreshMenu()
    }

    private func idleStateDidChange() {
        applyIdleState()
        refreshMenu()
    }

    private func applyIdleState() {
        guard settings.idleOnlyMode else { return }
        let shouldPause = idleDetector.state == .active
        workerControl.setPaused(shouldPause, port: settings.statusPort)
    }

    private func pollerDidUpdate(_ state: PollerState) {
        refreshMenu()
    }

    private func workerDidChangeState(_ state: WorkerProcessState) {
        // Re-apply pause state after worker restarts so the new process is paused if needed.
        if case .running = state {
            applyIdleState()
        }
        refreshMenu()
    }

    private func refreshMenu() {
        menuController.update(
            pollerState: statusPoller.state,
            workerState: workerProcess?.state ?? .idle,
            idleOnlyMode: settings.idleOnlyMode,
            userActive: idleDetector.state == .active
        )
    }
}
