import AppKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    private var statusItem: NSStatusItem!
    private var menuController: MenuController!
    private var workerProcess: WorkerProcess?
    private var statusPoller: StatusPoller!
    private var settingsWindowController: SettingsWindowController?
    private let store = SettingsStore()
    private var settings: Settings = Settings()
    private var token: String = ""

    func applicationDidFinishLaunching(_ notification: Notification) {
        settings = store.load()
        token = store.loadToken()

        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            button.title = "⬡"
            button.font = NSFont.systemFont(ofSize: 14)
        }

        menuController = MenuController(statusItem: statusItem)
        wireMenuActions()

        statusPoller = StatusPoller(port: settings.statusPort)
        statusPoller.onStateChange = { [weak self] state in
            DispatchQueue.main.async { self?.pollerDidUpdate(state) }
        }

        migrateIfNeeded()
        startWorkerIfNeeded()
        statusPoller.start()
    }

    func applicationWillTerminate(_ notification: Notification) {
        workerProcess?.stop()
    }

    // MARK: Private

    private func wireMenuActions() {
        menuController.onStartWorker = { [weak self] in self?.startWorkerIfNeeded() }
        menuController.onStopWorker = { [weak self] in self?.workerProcess?.stop() }
        menuController.onToggleP2P = { [weak self] in self?.toggleP2P() }
        menuController.onOpenSettings = { [weak self] in self?.openSettings() }
        menuController.onOpenLogs = { [weak self] in self?.openLogs() }
        menuController.onToggleLaunchAtLogin = { [weak self] in self?.toggleLaunchAtLogin() }
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
            workerProcess?.restart(env: settings.envVars(token: token))
            return
        }
        guard let binaryURL = WorkerProcess.resolveWorkerBinary() else {
            let alert = NSAlert()
            alert.messageText = "comrad-worker not found"
            alert.informativeText = "Could not locate the worker binary inside COMRAD.app.\nPlease reinstall."
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
        proc.start(env: settings.envVars(token: token))
    }

    private func toggleP2P() {
        settings.disableP2P.toggle()
        try? store.save(settings)
        workerProcess?.restart(env: settings.envVars(token: token))
    }

    private func openSettings() {
        if settingsWindowController == nil {
            settingsWindowController = SettingsWindowController(store: store) { [weak self] newSettings, newToken in
                guard let self else { return }
                let changed = newSettings != self.settings || newToken != self.token
                self.settings = newSettings
                self.token = newToken
                self.statusPoller.updatePort(newSettings.statusPort)
                if changed {
                    self.workerProcess?.restart(env: self.settings.envVars(token: self.token))
                }
                self.settingsWindowController = nil
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

    private func pollerDidUpdate(_ state: PollerState) {
        refreshMenu()
    }

    private func workerDidChangeState(_ state: WorkerProcessState) {
        refreshMenu()
    }

    private func refreshMenu() {
        menuController.update(
            pollerState: statusPoller.state,
            workerState: workerProcess?.state ?? .idle,
            p2pDisabled: settings.disableP2P
        )
    }
}
