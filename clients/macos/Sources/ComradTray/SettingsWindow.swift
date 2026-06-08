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

    private let maxMemoryGB: Int
    private let maxDiskGB: Int
    private let store: SettingsStore
    private let initial: Settings
    private let onSave: (Settings, String) -> Void
    private let onDismiss: () -> Void

    init(store: SettingsStore,
         onSave: @escaping (Settings, String) -> Void,
         onDismiss: @escaping () -> Void) {
        let s = store.load()
        _managerURL = State(initialValue: s.managerURL)
        _token      = State(initialValue: store.loadToken())
        _memoryGB    = State(initialValue: s.memoryGB)
        _diskGB      = State(initialValue: s.diskGB)
        _p2pEnabled  = State(initialValue: !s.disableP2P)
        _p2pPortText = State(initialValue: "\(s.p2pPort)")
        self.maxMemoryGB = Settings.physicalMemoryGB()
        self.maxDiskGB   = Settings.availableDiskGB()
        self.store   = store
        self.initial = s
        self.onSave  = onSave
        self.onDismiss = onDismiss
    }

    var body: some View {
        VStack(spacing: 0) {
            VStack(alignment: .leading, spacing: 14) {
                section("Connection", icon: "network") {
                    row("Manager URL",
                        help: "URL COMRAD Manager, к которому подключается воркер для получения задач. Пример: http://192.168.1.100:1922") {
                        TextField("http://127.0.0.1:1922", text: $managerURL)
                            .font(.system(.body, design: .monospaced))
                    }
                    row("Token",
                        help: "Секретный токен для авторизации воркера у менеджера. Находится в настройках менеджера.") {
                        SecureField("worker token", text: $token)
                    }
                }
                section("Worker", icon: "cpu") {
                    resourceRow("Memory", value: $memoryGB, max: maxMemoryGB,
                                help: "Unified-память, предоставляемая кластеру для загрузки моделей. Типичный размер модели: 7B ≈ 4 GB, 13B ≈ 8 GB, 70B ≈ 32 GB.")
                    resourceRow("Disk", value: $diskGB, max: maxDiskGB,
                                help: "Максимальный объём диска для кэша моделей. Воркер не будет скачивать модели сверх этого лимита.")
                }
                section("P2P", icon: "wifi") {
                    row("Enable P2P",
                        help: "P2P-раздача позволяет нодам обмениваться файлами моделей напрямую, не скачивая их заново с внешних серверов.") {
                        Toggle("", isOn: $p2pEnabled).labelsHidden()
                    }
                    row("Port",
                        help: "TCP/UDP порт для BitTorrent-обмена файлами моделей между нодами. Откройте его во входящих правилах файрволла для лучшей связности.") {
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
                Button("Cancel", action: onDismiss)
                    .keyboardShortcut(.cancelAction)
                Spacer()
                Button("Save", action: commitSave)
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
        .frame(width: 420)
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
            Text("GB").foregroundStyle(.secondary)
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
        try? store.save(s)
        try? store.saveToken(token)
        onSave(s, token)
        onDismiss()
    }
}

// MARK: - Window controller

final class SettingsWindowController: NSWindowController, NSWindowDelegate {
    init(store: SettingsStore, onSave: @escaping (Settings, String) -> Void) {
        let win = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 420, height: 300),
            styleMask: [.titled, .closable],
            backing: .buffered,
            defer: false
        )
        win.title = "COMRAD Settings"
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
