import AppKit

final class MenuController: NSObject, NSMenuDelegate {
    private weak var statusItem: NSStatusItem?
    private var menu: NSMenu = NSMenu()

    private var statusHeaderItem: NSMenuItem!
    private var startStopItem: NSMenuItem!
    private var launchAtLoginItem: NSMenuItem!
    private var idleOnlyItem: NSMenuItem!
    private var workerRunning = false

    var onStartWorker: (() -> Void)?
    var onStopWorker: (() -> Void)?
    var onOpenSettings: (() -> Void)?
    var onOpenLogs: (() -> Void)?
    var onToggleLaunchAtLogin: (() -> Void)?
    var onToggleIdleOnlyMode: (() -> Void)?
    var onQuit: (() -> Void)?

    init(statusItem: NSStatusItem) {
        self.statusItem = statusItem
        super.init()
        buildMenu()
        statusItem.menu = menu
        menu.delegate = self
    }

    private func makeItem(_ title: String,
                          systemImage: String? = nil,
                          action: Selector,
                          keyEquivalent: String = "") -> NSMenuItem {
        let item = NSMenuItem(title: title, action: action, keyEquivalent: keyEquivalent)
        item.target = self
        if let name = systemImage {
            item.image = NSImage(systemSymbolName: name, accessibilityDescription: nil)
        }
        return item
    }

    private func buildMenu() {
        statusHeaderItem = NSMenuItem(title: "Connecting…", action: nil, keyEquivalent: "")
        statusHeaderItem.isEnabled = false
        statusHeaderItem.image = statusDot(color: .tertiaryLabelColor)
        menu.addItem(statusHeaderItem)
        menu.addItem(.separator())

        startStopItem = makeItem("Start Worker", systemImage: "play.fill", action: #selector(startStop))
        menu.addItem(startStopItem)

        menu.addItem(.separator())
        menu.addItem(makeItem("Settings…",
                              systemImage: "gearshape",
                              action: #selector(openSettings),
                              keyEquivalent: ","))
        menu.addItem(makeItem("Open Logs", systemImage: "doc.text", action: #selector(openLogs)))
        menu.addItem(.separator())

        launchAtLoginItem = makeItem("Launch at Login",
                                    systemImage: "arrow.up.right.square",
                                    action: #selector(toggleLaunchAtLogin))
        launchAtLoginItem.state = LaunchAtLogin.isRegistered ? .on : .off
        menu.addItem(launchAtLoginItem)

        idleOnlyItem = makeItem("Work Only When Idle",
                                systemImage: "moon",
                                action: #selector(toggleIdleOnlyMode))
        idleOnlyItem.state = .off
        menu.addItem(idleOnlyItem)

        menu.addItem(.separator())
        menu.addItem(makeItem("Quit COMRAD", action: #selector(quit), keyEquivalent: "q"))
    }

    func update(pollerState: PollerState,
                workerState: WorkerProcessState,
                idleOnlyMode: Bool,
                userActive: Bool) {
        idleOnlyItem.state = idleOnlyMode ? .on : .off
        let color = statusColor(pollerState: pollerState, idleOnlyMode: idleOnlyMode, userActive: userActive)
        let alpha: CGFloat = isVisibleToManager(pollerState: pollerState) ? 1.0 : 0.6
        updateStatusHeader(pollerState: pollerState, idleOnlyMode: idleOnlyMode, userActive: userActive, color: color)
        updateTrayIcon(color: color, alpha: alpha)
        updateWorkerToggle(workerState: workerState)
        launchAtLoginItem.state = LaunchAtLogin.isRegistered ? .on : .off
    }

    // MARK: Private update helpers

    private func isVisibleToManager(pollerState: PollerState) -> Bool {
        if case .connected(let snap) = pollerState { return snap.connected }
        return false
    }

    private func statusColor(pollerState: PollerState, idleOnlyMode: Bool, userActive: Bool) -> NSColor {
        if idleOnlyMode && userActive { return .systemOrange }
        switch pollerState {
        case .connected(let snap):
            guard snap.connected else { return .systemOrange }
            return snap.slots.filter { $0.state == "busy" }.isEmpty ? .systemGreen : .systemBlue
        case .error:   return .systemRed
        case .connecting, .stopped: return .tertiaryLabelColor
        }
    }

    private func updateTrayIcon(color: NSColor, alpha: CGFloat) {
        guard let button = statusItem?.button else { return }
        button.image = makeTrayImage(color: color, alpha: alpha)
    }

    private func makeTrayImage(color: NSColor, alpha: CGFloat) -> NSImage {
        let size: CGFloat = 18
        let dotSize: CGFloat = 6

        let image = NSImage(size: NSSize(width: size, height: size), flipped: false) { rect in
            // Icon alpha baked into the symbol color so NSImage.draw respects it
            let iconColor = NSColor.labelColor.withAlphaComponent(alpha)
            let cfg = NSImage.SymbolConfiguration(paletteColors: [iconColor])
            if let cpu = NSImage(systemSymbolName: "cpu", accessibilityDescription: nil)?
                .withSymbolConfiguration(cfg) {
                cpu.draw(in: rect)
            }
            // Indicator dot: always fully opaque
            color.setFill()
            NSBezierPath(ovalIn: NSRect(x: rect.maxX - dotSize, y: rect.minY,
                                        width: dotSize, height: dotSize)).fill()
            return true
        }
        image.isTemplate = false
        return image
    }

    private func updateStatusHeader(pollerState: PollerState, idleOnlyMode: Bool, userActive: Bool, color: NSColor) {
        statusHeaderItem.image = statusDot(color: color)

        if idleOnlyMode && userActive {
            statusHeaderItem.title = "Paused — screen in use"
            return
        }

        switch pollerState {
        case .connected(let snap):
            let name = snap.nodeName.isEmpty ? snap.nodeId : snap.nodeName
            if snap.connected {
                let busy = snap.slots.filter { $0.state == "busy" }.count
                statusHeaderItem.title = "\(name) — \(busy > 0 ? "Working…" : "Ready")"
            } else {
                statusHeaderItem.title = humanizeConnectionError(snap.lastError)
            }
        case .error:
            statusHeaderItem.title = "Worker not responding"
        case .connecting:
            statusHeaderItem.title = "Starting up…"
        case .stopped:
            statusHeaderItem.title = "Worker is stopped"
        }
    }

    private func humanizeConnectionError(_ error: String?) -> String {
        guard let error else { return "Disconnected from manager" }
        if error.contains("i/o timeout") || error.contains("timed out") {
            return "Manager is unreachable"
        }
        if error.contains("connection refused") {
            return "Manager is not running"
        }
        if error.contains("no such host") || error.contains("no route to host") {
            return "Manager host not found"
        }
        return "Disconnected from manager"
    }

    private func updateWorkerToggle(workerState: WorkerProcessState) {
        switch workerState {
        case .running, .restarting:
            workerRunning = true
            startStopItem.title = "Stop Worker"
            startStopItem.image = NSImage(systemSymbolName: "stop.fill",
                                          accessibilityDescription: nil)
        default:
            workerRunning = false
            startStopItem.title = "Start Worker"
            startStopItem.image = NSImage(systemSymbolName: "play.fill",
                                          accessibilityDescription: nil)
        }
    }

    private func statusDot(color: NSColor) -> NSImage? {
        NSImage(systemSymbolName: "circle.fill", accessibilityDescription: nil)?
            .withSymbolConfiguration(.init(paletteColors: [color]))
    }

    // MARK: Actions

    @objc private func startStop() {
        workerRunning ? onStopWorker?() : onStartWorker?()
    }

    @objc private func openSettings() { onOpenSettings?() }
    @objc private func openLogs() { onOpenLogs?() }
    @objc private func toggleLaunchAtLogin() { onToggleLaunchAtLogin?() }
    @objc private func toggleIdleOnlyMode() { onToggleIdleOnlyMode?() }
    @objc private func quit() { onQuit?() }
}
