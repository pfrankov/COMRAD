import Foundation

struct Localization {
    static var shared: Localization = Localization(language: "en")

    private let messages: [String: Any]
    private let fallbackMessages: [String: Any]

    init(language: String) {
        self.messages = Self.loadMessages(language) ?? [:]
        self.fallbackMessages = language != "en" ? (Self.loadMessages("en") ?? [:]) : [:]
    }

    init(messages: [String: Any]) {
        self.messages = messages
        self.fallbackMessages = [:]
    }

    func t(_ key: String, values: [String: Any]? = nil, fallback: String? = nil) -> String {
        let template = resolveKey(key, in: messages) ?? resolveKey(key, in: fallbackMessages) ?? fallback ?? key
        guard let templateStr = template as? String else { return key }
        return interpolate(templateStr, values: values)
    }

    private func resolveKey(_ key: String, in dict: [String: Any]) -> Any? {
        let parts = key.split(separator: ".")
        var current: Any = dict
        for part in parts {
            guard let d = current as? [String: Any], let value = d[String(part)] else {
                return nil
            }
            current = value
        }
        return current
    }

    private func interpolate(_ string: String, values: [String: Any]?) -> String {
        guard let values else { return string }
        var result = string
        for (key, value) in values {
            result = result.replacingOccurrences(of: "{\(key)}", with: "\(value)")
        }
        return result
    }

    private static func loadMessages(_ language: String) -> [String: Any]? {
        let candidates: [Bundle] = [.module, .main]
        for bundle in candidates {
            if let url = bundle.url(forResource: "translations/\(language)", withExtension: "json"),
               let data = try? Data(contentsOf: url),
               let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                return json
            }
        }
        if let resourceURL = Bundle.main.resourceURL {
            let url = resourceURL.appendingPathComponent("translations/\(language).json")
            if let data = try? Data(contentsOf: url),
               let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
                return json
            }
        }
        return nil
    }

    static let locales: [String] = ["en", "zh", "es", "fr", "ru", "de", "ja", "pt"]

    static let localeNames: [String: String] = [
        "en": "English",
        "zh": "中文",
        "es": "Español",
        "fr": "Français",
        "ru": "Русский",
        "de": "Deutsch",
        "ja": "日本語",
        "pt": "Português",
    ]

    static func systemLanguage() -> String {
        let preferred = Locale.preferredLanguages.first ?? "en"
        let normalized = preferred.lowercased().replacingOccurrences(of: "_", with: "-")
        let base = normalized.split(separator: "-").first.map(String.init) ?? "en"
        guard locales.contains(base) else { return "en" }
        return base
    }
}
