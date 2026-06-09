import AppKit
import SwiftUI

// MARK: - SwiftUI view

private struct SettingsView: View {
    @State private var managerURL: String
    @State private var token: String
    @State private var memoryGB: Int
    @State private var diskGB: Int
    @State private var p2pEnabled: Bool
    @State private var p2pPortText: String
    @State private var selectedLanguage: String

    private let maxMemoryGB: Int
    private let maxDiskGB: Int
    private let store: SettingsStore
    private let initial: Settings
    private let onSave: (Settings) -> Void
    private let onDismiss: () -> Void

    init(store: SettingsStore,
         onSave: @escaping (Settings) -> Void,
         onDismiss: @escaping () -> Void) {
        let s = store.load()
        _managerURL = State(initialValue: s.managerURL)
        _token      = State(initialValue: s.token)
        _memoryGB    = State(initialValue: s.memoryGB)
        _diskGB      = State(initialValue: s.diskGB)
        _p2pEnabled  = State(initialValue: !s.disableP2P)
        _p2pPortText = State(initialValue: "\(s.p2pPort)")
        _selectedLanguage = State(initialValue: s.language.isEmpty ? Localization.systemLanguage() : s.language)
        self.maxMemoryGB = Settings.physicalMemoryGB()
        self.maxDiskGB   = Settings.availableDiskGB()
        self.store   = store
        self.initial = s
        self.onSave  = onSave
        self.onDismiss = onDismiss
    }

    var body: some View {
        let loc = Localization.shared
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 14) {
                section(loc.t("settings.language"), icon: "globe") {
                    languagePicker(loc: loc)
                }
                section(loc.t("settings.connection"), icon: "network") {
                    row(loc.t("settings.managerUrl"),
                        help: loc.t("settings.managerUrlHelp")) {
                        TextField("http://127.0.0.1:1922", text: $managerURL)
                            .font(.system(.body, design: .monospaced))
                    }
                    row(loc.t("settings.token"),
                        help: loc.t("settings.tokenHelp")) {
                        SecureField(loc.t("settings.token"), text: $token)
                    }
                }
                section(loc.t("settings.worker"), icon: "cpu") {
                    resourceRow(loc.t("settings.memory"), value: $memoryGB, max: maxMemoryGB,
                                help: loc.t("settings.memoryHelp"))
                    resourceRow(loc.t("settings.disk"), value: $diskGB, max: maxDiskGB,
                                help: loc.t("settings.diskHelp"))
                }
                section(loc.t("settings.p2p"), icon: "wifi") {
                    row(loc.t("settings.enableP2P"),
                        help: loc.t("settings.enableP2PHelp")) {
                        Toggle("", isOn: $p2pEnabled).labelsHidden()
                    }
                    row(loc.t("settings.port"),
                        help: loc.t("settings.portHelp")) {
                        TextField("6881", text: $p2pPortText)
                            .frame(width: 70)
                            .multilineTextAlignment(.trailing)
                            .font(.system(.body, design: .monospaced))
                            .disabled(!p2pEnabled)
                    }
                }
            }
            .padding(20)

            Divider()

            HStack {
                Button(loc.t("settings.cancel"), action: onDismiss)
                    .keyboardShortcut(.cancelAction)
                Spacer()
                Button(loc.t("settings.save"), action: commitSave)
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
        .frame(width: 440)
    }

    @ViewBuilder
    private func languagePicker(loc: Localization) -> some View {
        HStack(spacing: 10) {
            Text(loc.t("settings.language"))
                .frame(width: 90, alignment: .trailing)
                .foregroundStyle(.secondary)
            Picker("", selection: $selectedLanguage) {
                ForEach(Localization.locales, id: \.self) { code in
                    Text(Localization.localeNames[code] ?? code).tag(code)
                }
            }
            .labelsHidden()
            .frame(width: 180)
            Spacer()
        }
    }

    @ViewBuilder
    private func resourceRow(_ label: String, value: Binding<Int>, max maxVal: Int,
                             help: String) -> some View {
        HStack(spacing: 8) {
            Text(label)
                .frame(width: 90, alignment: .trailing)
                .foregroundStyle(.secondary)
                .help(help)
            Slider(
                value: Binding(
                    get: { Double(value.wrappedValue.clamped(to: 1...maxVal)) },
                    set: { value.wrappedValue = Int($0.rounded()) }
                ),
                in: 1...Double(maxVal)
            )
            TextField("", value: value, format: .number)
                .frame(width: 48)
                .multilineTextAlignment(.trailing)
                .onChange(of: value.wrappedValue) { v in
                    if v > maxVal { value.wrappedValue = maxVal }
                    if v < 1     { value.wrappedValue = 1 }
                }
            Text(Localization.shared.t("settings.gb")).foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private func section<Content: View>(_ title: String, icon: String,
                                        @ViewBuilder content: () -> Content) -> some View {
        GroupBox {
            VStack(alignment: .leading, spacing: 10) { content() }
                .padding(.top, 2)
        } label: {
            Label(title, systemImage: icon).font(.subheadline.weight(.semibold))
        }
    }

    @ViewBuilder
    private func row<Content: View>(_ label: String, help: String = "",
                                    @ViewBuilder content: () -> Content) -> some View {
        HStack(spacing: 10) {
            Text(label)
                .frame(width: 90, alignment: .trailing)
                .foregroundStyle(.secondary)
                .help(help)
            content()
        }
    }

    private func commitSave() {
        var s = initial
        s.managerURL = managerURL
        s.memoryGB   = memoryGB.clamped(to: 1...maxMemoryGB)
        s.diskGB     = diskGB.clamped(to: 1...maxDiskGB)
        s.disableP2P = !p2pEnabled
        s.p2pPort    = Int(p2pPortText) ?? initial.p2pPort
        s.token = token
        s.language = selectedLanguage
        try? store.save(s)
        onSave(s)
        onDismiss()
    }
}

// MARK: - Window controller

final class SettingsWindowController: NSWindowController, NSWindowDelegate {
    init(store: SettingsStore, onSave: @escaping (Settings) -> Void) {
        let win = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 440, height: 300),
            styleMask: [.titled, .closable],
            backing: .buffered,
            defer: false
        )
        win.title = Localization.shared.t("settings.title")
        super.init(window: win)
        win.delegate = self

        let view = SettingsView(store: store, onSave: onSave) { [weak win] in win?.close() }
        let hosting = NSHostingView(rootView: view)
        win.contentView = hosting
        win.setContentSize(hosting.fittingSize)
        win.center()
    }

    required init?(coder: NSCoder) { fatalError() }
}

private extension Comparable {
    func clamped(to range: ClosedRange<Self>) -> Self {
        min(max(self, range.lowerBound), range.upperBound)
    }
}
