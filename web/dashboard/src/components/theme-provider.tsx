import * as React from "react"

export type Theme = "system" | "light" | "dark"
type ResolvedTheme = "light" | "dark"

type ThemeProviderProps = {
  children: React.ReactNode
  defaultTheme?: Theme
  storageKey?: string
}

type ThemeProviderState = {
  theme: Theme
  resolvedTheme: ResolvedTheme
  setTheme: (theme: Theme) => void
}

const COLOR_SCHEME_QUERY = "(prefers-color-scheme: dark)"
const THEME_VALUES: Theme[] = ["system", "light", "dark"]

const ThemeProviderContext = React.createContext<
  ThemeProviderState | undefined
>(undefined)

function isTheme(value: string | null): value is Theme {
  return value !== null && THEME_VALUES.includes(value as Theme)
}

function systemTheme(): ResolvedTheme {
  if (typeof window === "undefined") return "light"
  if (window.matchMedia(COLOR_SCHEME_QUERY).matches) return "dark"
  return "light"
}

function storedTheme(storageKey: string, fallback: Theme) {
  try {
    const stored = localStorage.getItem(storageKey)
    return isTheme(stored) ? stored : fallback
  } catch {
    return fallback
  }
}

export function ThemeProvider({
  children,
  defaultTheme = "system",
  storageKey = "theme",
}: ThemeProviderProps) {
  const [theme, setThemeState] = React.useState<Theme>(() =>
    storedTheme(storageKey, defaultTheme)
  )
  const [resolvedTheme, setResolvedTheme] = React.useState<ResolvedTheme>(() =>
    theme === "system" ? systemTheme() : theme
  )

  const setTheme = React.useCallback(
    (nextTheme: Theme) => {
      try {
        localStorage.setItem(storageKey, nextTheme)
      } catch {
        // Persistence is best-effort; applying the theme still works.
      }
      setThemeState(nextTheme)
    },
    [storageKey]
  )

  React.useEffect(() => {
    const nextResolved = theme === "system" ? systemTheme() : theme
    const root = document.documentElement

    root.classList.remove("light", "dark")
    root.classList.add(nextResolved)
    root.style.colorScheme = nextResolved
    setResolvedTheme(nextResolved)

    if (theme !== "system") return undefined

    const media = window.matchMedia(COLOR_SCHEME_QUERY)
    const onChange = () => {
      const resolved = systemTheme()
      root.classList.remove("light", "dark")
      root.classList.add(resolved)
      root.style.colorScheme = resolved
      setResolvedTheme(resolved)
    }

    media.addEventListener("change", onChange)
    return () => media.removeEventListener("change", onChange)
  }, [theme])

  React.useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.storageArea !== localStorage || event.key !== storageKey) return
      setThemeState(isTheme(event.newValue) ? event.newValue : defaultTheme)
    }

    window.addEventListener("storage", onStorage)
    return () => window.removeEventListener("storage", onStorage)
  }, [defaultTheme, storageKey])

  const value = React.useMemo(
    () => ({ theme, resolvedTheme, setTheme }),
    [theme, resolvedTheme, setTheme]
  )

  return (
    <ThemeProviderContext.Provider value={value}>
      {children}
    </ThemeProviderContext.Provider>
  )
}

export function useTheme() {
  const context = React.useContext(ThemeProviderContext)
  if (!context) throw new Error("useTheme must be used within ThemeProvider")
  return context
}
