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
import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'

import { describe, expect, it } from 'vitest'

const MOBILE_DIR = path.resolve('src/mobile')
const LOCALES_DIR = path.resolve('src/i18n/locales')
const LOCALES = ['en', 'zh', 'zh-TW', 'fr', 'ru', 'ja', 'vi'] as const

// Matches the literal `t('key')` forms used across the mobile console. Calls
// with a dynamic key (t(TAB_LABEL_KEY[tab])) are intentionally not matched.
const T_CALL = /\bt\(\s*['"`]([^'"`\n]+?)['"`]\s*[,)]/g
const T_CALL_MULTILINE = /\bt\(\s*['"`]([^'"`]+?)['"`]\s*\)/g

function collectSourceFiles(dir: string): string[] {
  const files: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === '__tests__' || entry.name === 'node_modules') continue
      files.push(...collectSourceFiles(fullPath))
      continue
    }
    if (/\.(tsx?|jsx?)$/.test(entry.name)) files.push(fullPath)
  }
  return files
}

function collectLiteralKeys(): Set<string> {
  const keys = new Set<string>()
  for (const file of collectSourceFiles(MOBILE_DIR)) {
    const content = readFileSync(file, 'utf8')
    for (const regex of [T_CALL, T_CALL_MULTILINE]) {
      regex.lastIndex = 0
      let match: RegExpExecArray | null
      while ((match = regex.exec(content)) !== null) {
        const key = match[1]
        if (key.includes('${') || key.startsWith('{{')) continue
        keys.add(key)
      }
    }
  }
  return keys
}

function readLocale(locale: string): Record<string, string> {
  const raw = readFileSync(path.join(LOCALES_DIR, `${locale}.json`), 'utf8')
  return (JSON.parse(raw) as { translation: Record<string, string> })
    .translation
}

describe('mobile i18n coverage', () => {
  it('has every literal t() key in the mobile console across all locales', () => {
    const keys = collectLiteralKeys()
    expect(keys.size).toBeGreaterThan(0)

    const report: Record<string, string[]> = {}
    for (const locale of LOCALES) {
      const translation = readLocale(locale)
      const missing = [...keys].filter((key) => !(key in translation)).sort()
      if (missing.length > 0) report[locale] = missing
    }

    expect(report).toEqual({})
  })
})
