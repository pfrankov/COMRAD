export const locales = ["en", "zh", "es", "fr", "ru", "de", "ja", "pt"] as const

export type Locale = (typeof locales)[number]
export type LanguagePreference = "system" | Locale

declare global {
  interface Window {
    __COMRAD_SYSTEM_LOCALE__?: string
  }
}

export const localeNames: Record<Locale, string> = {
  en: "English",
  zh: "中文",
  es: "Español",
  fr: "Français",
  ru: "Русский",
  de: "Deutsch",
  ja: "日本語",
  pt: "Português",
}

export function isLocale(value: string | null | undefined): value is Locale {
  return locales.includes(value as Locale)
}

export function isLanguagePreference(
  value: string | null | undefined
): value is LanguagePreference {
  return value === "system" || isLocale(value)
}

export function normalizeLocale(
  value: string | null | undefined
): Locale | null {
  if (!value) return null
  const normalized = value.toLowerCase().replace("_", "-")
  const base = normalized.split("-")[0]
  if (base === "zh") return "zh"
  if (base === "pt") return "pt"
  if (isLocale(base)) return base
  return null
}

export function detectLocale(languages?: readonly string[]): Locale {
  const primary = primaryNavigatorLocale(languages)
  if (primary) return primary
  const intl = intlLocale()
  if (intl) return intl
  const seeded = systemLocaleSeed()
  if (seeded) return seeded
  return "en"
}

export function systemLocaleSeed(): Locale | null {
  if (typeof window === "undefined") return null
  return normalizeLocale(window.__COMRAD_SYSTEM_LOCALE__)
}

function intlLocale(): Locale | null {
  if (typeof Intl === "undefined") return null
  return normalizeLocale(Intl.DateTimeFormat().resolvedOptions().locale)
}

function primaryNavigatorLocale(languages?: readonly string[]): Locale | null {
  const candidates =
    languages && languages.length ? languages : navigatorLocales()
  return normalizeLocale(candidates[0])
}

function navigatorLocales(): readonly string[] {
  if (typeof navigator === "undefined") return []
  return navigator.languages?.length
    ? navigator.languages
    : [navigator.language]
}
