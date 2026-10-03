/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { readdirSync, readFileSync, mkdirSync, writeFileSync } from 'node:fs'
import { join, relative } from 'node:path'
import { pathToFileURL } from 'node:url'

// The mobile console only ever renders a few dozen strings, while a shared
// locale file holds every desktop string (~7k keys). The console therefore gets
// its own trimmed locale files under public-mobile/locales, generated from the
// keys its sources actually reference. src/mobile/lib/i18n.ts fetches them at
// runtime, so no locale data is bundled into the mobile JavaScript.

/** Locale files written for the console, keyed by interface language code. */
export const MOBILE_LOCALE_FILES = {
  en: 'en.json',
  zhCN: 'zh.json',
  zhTW: 'zh-TW.json',
  fr: 'fr.json',
  ru: 'ru.json',
  ja: 'ja.json',
  vi: 'vi.json',
}

const SOURCE_EXTENSIONS = /\.(tsx?|jsx?)$/
const T_CALL_SINGLE_LINE = /\bt\(\s*['"]([^'"\n]+?)['"]\s*[,)]/g
const T_CALL_MULTI_LINE = /\bt\(\s*['"]([^'"\n]+?)['"]\s*\)/g
const STRING_LITERAL = /'([^'\\\n]+)'|"([^"\\\n]+)"/g

function collectSourceFiles(dir) {
  const files = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === '__tests__' || entry.name === 'node_modules') continue
      files.push(...collectSourceFiles(fullPath))
      continue
    }
    if (SOURCE_EXTENSIONS.test(entry.name)) files.push(fullPath)
  }
  return files
}

/**
 * Collects the translation keys the mobile console can render:
 *
 * 1. every literal `t('key')` call, which is the authoritative set, and
 * 2. every other string literal that happens to be a known locale key, so the
 *    lookups that go through a label map or a prop (`t(TAB_LABEL_KEY[tab])`,
 *    `t(props.label)`) are covered without hand-maintaining a list.
 *
 * @param {Array<[string, string]>} sources - `[path, content]` pairs.
 * @param {Set<string>} englishKeys - keys present in the base locale.
 */
export function collectMobileLocaleKeys(sources, englishKeys) {
  const literalKeys = new Set()
  const candidateKeys = new Set()

  for (const [, content] of sources) {
    for (const regex of [T_CALL_SINGLE_LINE, T_CALL_MULTI_LINE]) {
      regex.lastIndex = 0
      let match
      while ((match = regex.exec(content)) !== null) {
        const key = match[1]
        if (key.includes('${') || key.startsWith('{{')) continue
        literalKeys.add(key)
      }
    }

    STRING_LITERAL.lastIndex = 0
    let literal
    while ((literal = STRING_LITERAL.exec(content)) !== null) {
      const value = literal[1] ?? literal[2]
      if (value && englishKeys.has(value)) candidateKeys.add(value)
    }
  }

  return {
    literalKeys: [...literalKeys].sort(),
    keys: [...new Set([...literalKeys, ...candidateKeys])].sort(),
  }
}

/**
 * Fails the build when a literal `t('key')` call has no base translation, which
 * would otherwise surface as an untranslated key in the console.
 */
export function assertLiteralsCovered(literalKeys, englishKeys) {
  const missing = literalKeys.filter((key) => !englishKeys.has(key))
  if (missing.length > 0) {
    throw new Error(
      `Missing base translations for ${missing.length} mobile key(s): ${missing.join(', ')}`
    )
  }
}

/** Projects the full locale map onto the keys the console renders. */
export function subsetLocale(localeMap, keys) {
  const subset = {}
  for (const key of keys) {
    const value = localeMap[key]
    if (typeof value === 'string') subset[key] = value
  }
  return subset
}

export async function buildMobileLocales({ root, outDir: targetDir }) {
  const localesDir = join(root, 'src', 'i18n', 'locales')
  const sourceFiles = collectSourceFiles(join(root, 'src', 'mobile'))

  const localeMaps = {}
  for (const file of Object.values(MOBILE_LOCALE_FILES)) {
    const parsed = JSON.parse(readFileSync(join(localesDir, file), 'utf8'))
    localeMaps[file] = parsed.translation ?? parsed
  }

  const englishKeys = new Set(Object.keys(localeMaps[MOBILE_LOCALE_FILES.en]))
  const sources = sourceFiles.map((file) => [
    relative(root, file),
    readFileSync(file, 'utf8'),
  ])
  const { keys, literalKeys } = collectMobileLocaleKeys(sources, englishKeys)
  assertLiteralsCovered(literalKeys, englishKeys)

  const outDir = targetDir ?? join(root, 'public-mobile', 'locales')
  mkdirSync(outDir, { recursive: true })

  const report = []
  for (const [code, file] of Object.entries(MOBILE_LOCALE_FILES)) {
    const subset = subsetLocale(localeMaps[file], keys)
    const body = `${JSON.stringify(subset, null, 2)}\n`
    writeFileSync(join(outDir, file), body)
    report.push({
      code,
      file,
      keys: Object.keys(subset).length,
      bytes: Buffer.byteLength(body),
    })
  }

  return { keys: keys.length, literalKeys: literalKeys.length, report }
}

const isDirectRun =
  process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href

if (isDirectRun) {
  buildMobileLocales({ root: process.cwd() })
    .then(({ keys, literalKeys, report }) => {
      console.log(
        `mobile locales: ${keys} keys (${literalKeys} literal t() calls)`
      )
      for (const entry of report) {
        console.log(
          `  ${entry.file.padEnd(10)} ${String(entry.keys).padStart(3)} keys  ${entry.bytes} B`
        )
      }
    })
    .catch((error) => {
      console.error(error.message)
      process.exit(1)
    })
}
