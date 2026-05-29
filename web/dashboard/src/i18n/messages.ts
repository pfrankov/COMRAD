import de from "@/i18n/messages/de.json"
import en from "@/i18n/messages/en.json"
import es from "@/i18n/messages/es.json"
import fr from "@/i18n/messages/fr.json"
import ja from "@/i18n/messages/ja.json"
import pt from "@/i18n/messages/pt.json"
import ru from "@/i18n/messages/ru.json"
import zh from "@/i18n/messages/zh.json"
import type { Locale } from "@/i18n/config"

export type Messages = Record<string, string>

export const messages: Record<Locale, Messages> = {
  en,
  zh,
  es,
  fr,
  ru,
  de,
  ja,
  pt,
}
