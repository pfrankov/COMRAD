import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react"

import {
  detectLocale,
  isLanguagePreference,
  localeNames,
  type LanguagePreference,
  type Locale,
} from "@/i18n/config"
import { messages } from "@/i18n/messages"

type Values = Record<string, string | number | boolean | null | undefined>
export type TFunction = (
  key: string,
  values?: Values,
  fallback?: string
) => string

type I18nContextValue = {
  language: LanguagePreference
  resolvedLocale: Locale
  setLanguage: (language: LanguagePreference) => void
  t: TFunction
}

const fallbackLocale: Locale = "en"
let currentLocale: Locale = fallbackLocale

const I18nContext = createContext<I18nContextValue | null>(null)

export function translate(key: string, values?: Values, fallback?: string) {
  return formatMessage(messageFor(currentLocale, key, fallback), values)
}

export function I18nProvider({
  children,
  defaultLanguage = "system",
  storageKey = "comrad.locale",
}: {
  children: ReactNode
  defaultLanguage?: LanguagePreference
  storageKey?: string
}) {
  const [language, setLanguageState] = useState<LanguagePreference>(() => {
    const stored = localStorage.getItem(storageKey)
    return isLanguagePreference(stored) ? stored : defaultLanguage
  })
  const [systemLocale, setSystemLocale] = useState(() => detectLocale())
  const resolvedLocale = language === "system" ? systemLocale : language

  useEffect(() => {
    currentLocale = resolvedLocale
    document.documentElement.lang = resolvedLocale
  }, [resolvedLocale])

  useEffect(() => {
    const syncSystemLocale = () =>
      setSystemLocale(detectLocale(navigator.languages))
    window.addEventListener("languagechange", syncSystemLocale)
    return () => window.removeEventListener("languagechange", syncSystemLocale)
  }, [])

  const setLanguage = useCallback(
    (nextLanguage: LanguagePreference) => {
      setLanguageState(nextLanguage)
      localStorage.setItem(storageKey, nextLanguage)
    },
    [storageKey]
  )

  const t = useCallback(
    (key: string, values?: Values, fallback?: string) =>
      formatMessage(messageFor(resolvedLocale, key, fallback), values),
    [resolvedLocale]
  )

  const value = useMemo(
    () => ({ language, resolvedLocale, setLanguage, t }),
    [language, resolvedLocale, setLanguage, t]
  )

  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n() {
  const context = useContext(I18nContext)
  if (!context) throw new Error("useI18n must be used within I18nProvider")
  return context
}

export function localeDisplayName(locale: Locale) {
  return localeNames[locale]
}

function messageFor(locale: Locale, key: string, fallback?: string) {
  return (
    messages[locale]?.[key] || messages[fallbackLocale][key] || fallback || key
  )
}

function formatMessage(message: string, values?: Values) {
  if (!values) return message
  return message.replace(/\{(\w+)\}/g, (match, name: string) => {
    const value = values[name]
    return value === undefined || value === null ? match : String(value)
  })
}
