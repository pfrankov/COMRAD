import AppKit

final class MenuController: NSObject, NSMenuDelegate {
    private weak var statusItem: NSStatusItem?
    private var menu: NSMenu = NSMenu()

    private var statusHeaderItem: NSMenuItem!
    private var startStopItem: NSMenuItem!
    private var p2pToggleItem: NSMenuItem!
    private var launchAtLoginItem: NSMenuItem!
    private var workerRunning = false

    var onStartWorker: (() -> Void)?
    var onStopWorker: (() -> Void)?
    var onToggleP2P: (() -> Void)?
    var onOpenSettings: (() -> Void)?
    var onOpenLogs: (() -> Void)?
    var onToggleLaunchAtLogin: (() -> Void)?
    var onQuit: (() -> Void)?

    init(statusItem: NSStatusItem) {
        self.statusItem = statusItem
        super.init()
        buildMenu()
        statusItem.menu = menu
        menu.delegate = self
    }

    private func makeItem(_ title: String, action: Selector, keyEquivalent: String = "") -> NSMenuItem {
        let item = NSMenuItem(title: title, action: action, keyEquivalent: keyEquivalent)
        item.target = self
        return item
    }

    private func buildMenu() {
        statusHeaderItem = NSMenuItem(title: "○ COMRAD Worker", action: nil, keyEquivalent: "")
        statusHeaderItem.isEnabled = false
        menu.addItem(statusHeaderItem)
        menu.addItem(.separator())

        startStopItem = makeItem("Start Worker", action: #selector(startStop))
        menu.addItem(startStopItem)

        p2pToggleItem = makeItem("Disable P2P", action: #selector(toggleP2P))
        menu.addItem(p2pToggleItem)

        menu.addItem(.separator())
        menu.addItem(makeItem("Settings…", action: #selector(openSettings), keyEquivalent: ","))
        menu.addItem(makeItem("Open Logs", action: #selector(openLogs)))
        menu.addItem(.separator())

        launchAtLoginItem = makeItem("Launch at Login", action: #selector(toggleLaunchAtLogin))
        launchAtLoginItem.state = LaunchAtLogin.isRegistered ? .on : .off
        menu.addItem(launchAtLoginItem)

        menu.addItem(.separator())
        menu.addItem(makeItem("Quit COMRAD", action: #selector(quit), keyEquivalent: "q"))
    }

    func update(pollerState: PollerState, workerState: WorkerProcessState, p2pDisabled: Bool) {
        switch pollerState {
        case .connected(let snap):
            let name = snap.nodeName.isEmpty ? snap.nodeId : snap.nodeName
            if snap.connected {
                statusHeaderItem.title = "● \(name) v\(snap.version)"
            } else {
                statusHeaderItem.title = "○ Disconnected — \(snap.lastError ?? "unknown")"
            }
        case .error(let msg):
            statusHeaderItem.title = "○ Error: \(msg)"
        case .connecting:
            statusHeaderItem.title = "○ Connecting…"
        case .stopped:
            statusHeaderItem.title = "○ Worker stopped"
        }

        switch workerState {
        case .running, .restarting:
            workerRunning = true
            startStopItem.title = "Stop Worker"
        default:
            workerRunning = false
            startStopItem.title = "Start Worker"
        }

        p2pToggleItem.title = p2pDisabled ? "Enable P2P" : "Disable P2P"
        launchAtLoginItem.state = LaunchAtLogin.isRegistered ? .on : .off
    }

    // MARK: Actions

    @objc private func startStop() {
        workerRunning ? onStopWorker?() : onStartWorker?()
    }

    @objc private func toggleP2P() { onToggleP2P?() }
    @objc private func openSettings() { onOpenSettings?() }
    @objc private func openLogs() { onOpenLogs?() }
    @objc private func toggleLaunchAtLogin() { onToggleLaunchAtLogin?() }
    @objc private func quit() { onQuit?() }
}
