import AppKit

final class SettingsWindowController: NSWindowController, NSWindowDelegate {
    private let store: SettingsStore
    private var settings: Settings
    private var token: String
    private var onSave: ((Settings, String) -> Void)?

    // Fields
    private let managerURLField = NSTextField()
    private let tokenField = NSSecureTextField()
    private let slotCountField = NSStepper()
    private let slotCountLabel = NSTextField(labelWithString: "1")
    private let statusPortField = NSTextField()
    private let p2pPortField = NSTextField()
    private let disableP2PCheck = NSButton(checkboxWithTitle: "Disable P2P", target: nil, action: nil)

    init(store: SettingsStore, onSave: @escaping (Settings, String) -> Void) {
        self.store = store
        self.settings = store.load()
        self.token = store.loadToken()
        self.onSave = onSave

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 420, height: 340),
            styleMask: [.titled, .closable],
            backing: .buffered,
            defer: false
        )
        window.title = "COMRAD Settings"
        window.center()
        super.init(window: window)
        window.delegate = self
        buildUI()
        loadValues()
    }

    required init?(coder: NSCoder) { fatalError() }

    private func buildUI() {
        guard let contentView = window?.contentView else { return }
        contentView.wantsLayer = true

        let grid = NSGridView()
        grid.translatesAutoresizingMaskIntoConstraints = false
        contentView.addSubview(grid)

        NSLayoutConstraint.activate([
            grid.leadingAnchor.constraint(equalTo: contentView.leadingAnchor, constant: 20),
            grid.trailingAnchor.constraint(equalTo: contentView.trailingAnchor, constant: -20),
            grid.topAnchor.constraint(equalTo: contentView.topAnchor, constant: 20),
        ])

        func label(_ text: String) -> NSTextField {
            let f = NSTextField(labelWithString: text)
            f.alignment = .right
            return f
        }

        managerURLField.placeholderString = "http://127.0.0.1:1922"
        managerURLField.identifier = NSUserInterfaceItemIdentifier("managerURL")
        tokenField.placeholderString = "worker token"
        tokenField.identifier = NSUserInterfaceItemIdentifier("token")
        statusPortField.placeholderString = "1923"
        statusPortField.identifier = NSUserInterfaceItemIdentifier("statusPort")
        p2pPortField.placeholderString = "6881"
        p2pPortField.identifier = NSUserInterfaceItemIdentifier("p2pPort")

        slotCountField.minValue = 1
        slotCountField.maxValue = 32
        slotCountField.valueWraps = false
        slotCountField.target = self
        slotCountField.action = #selector(slotCountChanged)
        slotCountField.identifier = NSUserInterfaceItemIdentifier("slotCount")

        let slotRow = NSStackView(views: [slotCountLabel, slotCountField])
        slotRow.orientation = .horizontal

        grid.addRow(with: [label("Manager URL:"), managerURLField])
        grid.addRow(with: [label("Worker Token:"), tokenField])
        grid.addRow(with: [label("Status Port:"), statusPortField])
        grid.addRow(with: [label("Slots:"), slotRow])
        grid.addRow(with: [label("P2P Port:"), p2pPortField])
        grid.addRow(with: [NSTextField(labelWithString: ""), disableP2PCheck])

        grid.column(at: 0).width = 110
        grid.column(at: 1).width = 260

        let saveButton = NSButton(title: "Save", target: self, action: #selector(save))
        saveButton.keyEquivalent = "\r"
        saveButton.translatesAutoresizingMaskIntoConstraints = false

        let cancelButton = NSButton(title: "Cancel", target: self, action: #selector(cancel))
        cancelButton.keyEquivalent = "\u{1b}"
        cancelButton.translatesAutoresizingMaskIntoConstraints = false

        let buttons = NSStackView(views: [cancelButton, saveButton])
        buttons.orientation = .horizontal
        buttons.translatesAutoresizingMaskIntoConstraints = false
        contentView.addSubview(buttons)
        NSLayoutConstraint.activate([
            buttons.trailingAnchor.constraint(equalTo: contentView.trailingAnchor, constant: -20),
            buttons.bottomAnchor.constraint(equalTo: contentView.bottomAnchor, constant: -20),
        ])
    }

    private func loadValues() {
        managerURLField.stringValue = settings.managerURL
        tokenField.stringValue = token
        statusPortField.stringValue = "\(settings.statusPort)"
        slotCountField.intValue = Int32(settings.slotCount)
        slotCountLabel.stringValue = "\(settings.slotCount)"
        p2pPortField.stringValue = "\(settings.p2pPort)"
        disableP2PCheck.state = settings.disableP2P ? .on : .off
    }

    @objc private func slotCountChanged() {
        slotCountLabel.stringValue = "\(slotCountField.intValue)"
    }

    @objc private func save() {
        settings.managerURL = managerURLField.stringValue
        settings.statusPort = Int(statusPortField.stringValue) ?? settings.statusPort
        settings.slotCount = Int(slotCountField.intValue)
        settings.p2pPort = Int(p2pPortField.stringValue) ?? settings.p2pPort
        settings.disableP2P = disableP2PCheck.state == .on
        let newToken = tokenField.stringValue
        try? store.save(settings)
        try? store.saveToken(newToken)
        onSave?(settings, newToken)
        close()
    }

    @objc private func cancel() {
        close()
    }
}
