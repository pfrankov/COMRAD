import AppKit

final class MenuController: NSObject, NSMenuDelegate {
    private weak var statusItem: NSStatusItem?
    private var menu: NSMenu = NSMenu()

    private var statusHeaderItem: NSMenuItem!
    private var statusDetailItem: NSMenuItem!
    private var startStopItem: NSMenuItem!
    private var launchAtLoginItem: NSMenuItem!
    private var idleOnlyItem: NSMenuItem!
    private var workerRunning = false

    private var pulseTimer: Timer?
    private var pulsePhase: CGFloat = 0
    private var pulseFadeOut: Int = -1  // -1 = normal, ≥0 = fade-out frame counter
    private var pulseFadeOutFrom: CGFloat = 1.0 // t value at fade-out start
    private var currentDotColor: NSColor = .tertiaryLabelColor
    private var currentIconAlpha: CGFloat = 1.0

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
        // Set the initial tray icon synchronously so the icon always appears
        // even before the first pulse timer tick.
        statusItem.button?.image = makeTrayImage(iconAlpha: 1.0, dotAlpha: 1.0)
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
        let loc = Localization.shared

        // Primary status line: colored dot + bold headline.
        statusHeaderItem = NSMenuItem(title: "", action: nil, keyEquivalent: "")
        statusHeaderItem.isEnabled = false
        statusHeaderItem.image = statusDot(color: .tertiaryLabelColor)
        menu.addItem(statusHeaderItem)

        // Secondary detail line: dimmed supporting info, hidden until populated.
        // A blank placeholder image of the same size as the dot aligns the text
        // column with the primary item without adding a visible glyph.
        statusDetailItem = NSMenuItem(title: "", action: nil, keyEquivalent: "")
        statusDetailItem.isEnabled = false
        statusDetailItem.image = blankImage(size: NSSize(width: 12, height: 12))
        statusDetailItem.isHidden = true
        menu.addItem(statusDetailItem)

        menu.addItem(.separator())

        startStopItem = makeItem(loc.t("menu.startWorker"), systemImage: "play.fill", action: #selector(startStop))
        menu.addItem(startStopItem)

        menu.addItem(.separator())
        menu.addItem(makeItem(loc.t("menu.settings"),
                              systemImage: "gearshape",
                              action: #selector(openSettings),
                              keyEquivalent: ","))
        menu.addItem(makeItem(loc.t("menu.openLogs"), systemImage: "doc.text", action: #selector(openLogs)))
        menu.addItem(.separator())

        launchAtLoginItem = makeItem(loc.t("menu.launchAtLogin"),
                                    systemImage: "arrow.up.right.square",
                                    action: #selector(toggleLaunchAtLogin))
        launchAtLoginItem.state = LaunchAtLogin.isRegistered ? .on : .off
        menu.addItem(launchAtLoginItem)

        idleOnlyItem = makeItem(loc.t("menu.idleOnly"),
                                systemImage: "moon",
                                action: #selector(toggleIdleOnlyMode))
        idleOnlyItem.state = .off
        menu.addItem(idleOnlyItem)

        menu.addItem(.separator())
        menu.addItem(makeItem(loc.t("menu.quit"), action: #selector(quit), keyEquivalent: "q"))
    }

    func update(pollerState: PollerState,
                workerState: WorkerProcessState,
                idleOnlyMode: Bool,
                userActive: Bool) {
        idleOnlyItem.state = idleOnlyMode ? .on : .off
        launchAtLoginItem.state = LaunchAtLogin.isRegistered ? .on : .off

        let line = makeStatusLine(
            pollerState: pollerState,
            workerState: workerState,
            idleOnlyMode: idleOnlyMode,
            userActive: userActive
        )

        applyStatusLine(line)

        currentDotColor = line.tone.color
        currentIconAlpha = isConnectedToManager(pollerState) ? 1.0 : 0.6
        redrawTrayIcon(dotAlpha: 1.0)

        if line.shouldPulse {
            startPulse()
        } else {
            stopPulse()
        }

        updateWorkerToggle(workerState: workerState)
    }

    // MARK: - Private update helpers

    private func applyStatusLine(_ line: StatusLine) {
        statusHeaderItem.image = statusDot(color: line.tone.color)
        statusHeaderItem.attributedTitle = NSAttributedString(
            string: line.primary,
            attributes: [
                .font: NSFont.systemFont(ofSize: 13, weight: .regular),
                .foregroundColor: NSColor.labelColor,
            ]
        )

        if let secondary = line.secondary {
            statusDetailItem.attributedTitle = NSAttributedString(
                string: secondary,
                attributes: [
                    .font: NSFont.systemFont(ofSize: 11),
                    .foregroundColor: NSColor.secondaryLabelColor,
                ]
            )
            statusDetailItem.isHidden = false
        } else {
            statusDetailItem.isHidden = true
        }
    }

    private func startPulse() {
        guard pulseTimer == nil else { return }
        pulsePhase = .pi / 2
        pulseFadeOut = -1
        pulseTimer = Timer.scheduledTimer(withTimeInterval: 1.0 / 24.0, repeats: true) { [weak self] _ in
            guard let self else { return }

            if self.pulseFadeOut >= 0 {
                // Fade-out mode — interpolate t from start value toward 1.0 over 10 frames
                self.pulseFadeOut += 1
                if self.pulseFadeOut > 10 {
                    self.pulseTimer?.invalidate()
                    self.pulseTimer = nil
                    self.redrawTrayIcon(dotAlpha: 1.0)
                    return
                }
                let progress = CGFloat(self.pulseFadeOut) / 10.0
                // ease-out cubic
                let eased = 3.0 * progress * progress - 2.0 * progress * progress * progress
                let t = self.pulseFadeOutFrom + (1.0 - self.pulseFadeOutFrom) * eased
                self.redrawTrayIcon(dotAlpha: 0.30 + 0.70 * t)
                return
            }

            // step ≈ 0.28 rad → period ≈ 2π/0.28/24 ≈ 0.93 s
            self.pulsePhase += 0.28
            let t = (sin(self.pulsePhase) + 1) / 2       // 0..1
            let dotAlpha = 0.30 + 0.70 * t               // 0.30..1.0
            self.redrawTrayIcon(dotAlpha: dotAlpha)
        }
    }

    private func stopPulse() {
        guard pulseTimer != nil, pulseFadeOut < 0 else { return }
        // Capture current t so fade-out interpolates from here to 1.0
        pulseFadeOutFrom = (sin(pulsePhase) + 1) / 2
        pulseFadeOut = 0  // begin the fade-out sequence
    }

    private func redrawTrayIcon(dotAlpha: CGFloat) {
        guard let button = statusItem?.button else { return }
        button.image = makeTrayImage(iconAlpha: currentIconAlpha, dotAlpha: dotAlpha)
    }

    private func makeTrayImage(iconAlpha: CGFloat, dotAlpha: CGFloat = 1.0) -> NSImage {
        let size: CGFloat = 18
        let dotSize: CGFloat = 6
        let color = currentDotColor

        let image = NSImage(size: NSSize(width: size, height: size), flipped: false) { rect in
            let iconColor = NSColor.labelColor.withAlphaComponent(iconAlpha)
            let cfg = NSImage.SymbolConfiguration(paletteColors: [iconColor])
            if let cpu = NSImage(systemSymbolName: "cpu", accessibilityDescription: nil)?
                .withSymbolConfiguration(cfg) {
                cpu.draw(in: rect)
            }
            color.withAlphaComponent(dotAlpha).setFill()
            NSBezierPath(ovalIn: NSRect(x: rect.maxX - dotSize, y: rect.minY,
                                        width: dotSize, height: dotSize)).fill()
            return true
        }
        image.isTemplate = false
        return image
    }

    private func updateWorkerToggle(workerState: WorkerProcessState) {
        let loc = Localization.shared
        switch workerState {
        case .running, .restarting:
            workerRunning = true
            startStopItem.title = loc.t("menu.stopWorker")
            startStopItem.image = NSImage(systemSymbolName: "stop.fill",
                                          accessibilityDescription: nil)
        default:
            workerRunning = false
            startStopItem.title = loc.t("menu.startWorker")
            startStopItem.image = NSImage(systemSymbolName: "play.fill",
                                          accessibilityDescription: nil)
        }
    }

    private func statusDot(color: NSColor) -> NSImage? {
        NSImage(systemSymbolName: "circle.fill", accessibilityDescription: nil)?
            .withSymbolConfiguration(.init(paletteColors: [color]))
    }

    private func blankImage(size: NSSize) -> NSImage {
        NSImage(size: size, flipped: false) { _ in true }
    }

    // MARK: - Actions

    @objc private func startStop() {
        workerRunning ? onStopWorker?() : onStartWorker?()
    }

    @objc private func openSettings() { onOpenSettings?() }
    @objc private func openLogs() { onOpenLogs?() }
    @objc private func toggleLaunchAtLogin() { onToggleLaunchAtLogin?() }
    @objc private func toggleIdleOnlyMode() { onToggleIdleOnlyMode?() }
    @objc private func quit() { onQuit?() }
}
