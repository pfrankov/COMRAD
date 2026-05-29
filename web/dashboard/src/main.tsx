import { StrictMode } from "react"
import { createRoot } from "react-dom/client"

import "./index.css"
import App from "./App.tsx"
import { ThemeProvider } from "@/components/theme-provider"
import { I18nProvider } from "@/i18n/i18n-provider"

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider defaultTheme="system" storageKey="comrad.theme">
      <I18nProvider defaultLanguage="system" storageKey="comrad.locale">
        <App />
      </I18nProvider>
    </ThemeProvider>
  </StrictMode>
)
