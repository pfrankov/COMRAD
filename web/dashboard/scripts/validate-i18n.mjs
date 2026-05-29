import fs from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..")
const srcDir = path.join(root, "src")
const messagesDir = path.join(srcDir, "i18n", "messages")
const locales = ["en", "zh", "es", "fr", "ru", "de", "ja", "pt"]
const keyPatterns = [
  /\b(?:t|tx|translate)\(\s*["'`]([A-Za-z0-9_.-]+)["'`]/g,
  /\b\w+Key:\s*["'`]([A-Za-z0-9_.-]+)["'`]/g,
]

const usedKeys = new Set()
for (const file of walk(srcDir)) {
  if (!/\.(ts|tsx)$/.test(file)) continue
  const text = fs.readFileSync(file, "utf8")
  for (const pattern of keyPatterns) {
    for (const match of text.matchAll(pattern)) usedKeys.add(match[1])
  }
}

const catalogs = new Map()
const missingCatalogs = []
const catalogKeys = new Set()
for (const locale of locales) {
  const file = path.join(messagesDir, `${locale}.json`)
  if (!fs.existsSync(file)) {
    missingCatalogs.push(locale)
    continue
  }
  const catalog = JSON.parse(fs.readFileSync(file, "utf8"))
  catalogs.set(locale, catalog)
  for (const key of Object.keys(catalog)) catalogKeys.add(key)
}

const missing = []
const empty = []
const unused = []
for (const key of [...usedKeys].sort()) {
  for (const locale of locales) {
    const catalog = catalogs.get(locale)
    if (!catalog || !(key in catalog)) {
      missing.push(`${locale}: ${key}`)
      continue
    }
    if (typeof catalog[key] !== "string" || !catalog[key].trim()) {
      empty.push(`${locale}: ${key}`)
    }
  }
}

for (const key of [...catalogKeys].sort()) {
  if (usedKeys.has(key)) continue
  for (const [locale, catalog] of catalogs) {
    if (key in catalog) unused.push(`${locale}: ${key}`)
  }
}

if (missingCatalogs.length || missing.length || empty.length || unused.length) {
  console.error("i18n validation failed")
  if (missingCatalogs.length) {
    console.error(`Missing catalogs: ${missingCatalogs.join(", ")}`)
  }
  if (missing.length) {
    console.error("Missing values:")
    for (const item of missing) console.error(`  - ${item}`)
  }
  if (empty.length) {
    console.error("Empty values:")
    for (const item of empty) console.error(`  - ${item}`)
  }
  if (unused.length) {
    console.error("Unused values:")
    for (const item of unused) console.error(`  - ${item}`)
  }
  process.exit(1)
}

console.log(`i18n validation passed (${usedKeys.size} keys).`)

function* walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (entry.name === "node_modules") continue
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) yield* walk(full)
    else yield full
  }
}
