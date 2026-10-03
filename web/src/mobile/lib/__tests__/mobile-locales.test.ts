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
import { mkdtempSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

// The generator is a build script, so it is imported dynamically by absolute
// path (nothing in src/ depends on scripts/) and `@vite-ignore` keeps the bundler
// from trying to resolve it.
const SCRIPT_PATH = join(process.cwd(), 'scripts', 'build-mobile-locales.mjs')

interface MobileLocalesModule {
  MOBILE_LOCALE_FILES: Record<string, string>
  assertLiteralsCovered: (
    literalKeys: string[],
    englishKeys: Set<string>
  ) => void
  buildMobileLocales: (options: { root: string; outDir?: string }) => Promise<{
    keys: number
    literalKeys: number
    report: Array<{ code: string; file: string; keys: number; bytes: number }>
  }>
  collectMobileLocaleKeys: (
    sources: Array<[string, string]>,
    englishKeys: Set<string>
  ) => { keys: string[]; literalKeys: string[] }
  subsetLocale: (
    localeMap: Record<string, string>,
    keys: string[]
  ) => Record<string, string>
}

async function loadScript(): Promise<MobileLocalesModule> {
  return (await import(/* @vite-ignore */ SCRIPT_PATH)) as MobileLocalesModule
}

describe('collectMobileLocaleKeys', () => {
  it('keeps literal t() calls and locale keys reached through a label map', async () => {
    const { collectMobileLocaleKeys } = await loadScript()
    const englishKeys = new Set(['Usage', 'Healthy', 'Mine'])
    const sources: Array<[string, string]> = [
      [
        'src/mobile/lib/example.ts',
        `
        const TAB_LABEL_KEY = { usage: 'Usage', models: 'Models by site' }
        export function label(tab, t) {
          return t(TAB_LABEL_KEY[tab])
        }
        export function page(t) {
          return t('Healthy') + t("Mine") + 'text-sm text-foreground'
        }
        `,
      ],
    ]

    const { keys, literalKeys } = collectMobileLocaleKeys(sources, englishKeys)

    expect(literalKeys).toEqual(['Healthy', 'Mine'])
    // 'Mine' is both a literal and a label-map value, 'Usage' only appears as a
    // map value, and the Tailwind classes are not locale keys.
    expect(keys).toEqual(['Healthy', 'Mine', 'Usage'])
  })

  it('ignores template interpolation and placeholder keys', async () => {
    const { collectMobileLocaleKeys } = await loadScript()
    const sources: Array<[string, string]> = [
      ['src/mobile/lib/example.ts', "t(`items: ${count}`); t('{{count}} ms')"],
    ]

    const { keys, literalKeys } = collectMobileLocaleKeys(
      sources,
      new Set(['items: 1'])
    )

    expect(literalKeys).toEqual([])
    expect(keys).toEqual([])
  })
})

describe('assertLiteralsCovered', () => {
  it('names every literal key without a base translation', async () => {
    const { assertLiteralsCovered } = await loadScript()

    expect(() =>
      assertLiteralsCovered(['Usage', 'Ghost'], new Set(['Usage']))
    ).toThrowError(/Ghost/)
  })

  it('accepts a fully translated set', async () => {
    const { assertLiteralsCovered } = await loadScript()

    expect(() =>
      assertLiteralsCovered(['Usage'], new Set(['Usage']))
    ).not.toThrow()
  })
})

describe('subsetLocale', () => {
  it('drops keys the target locale does not translate', async () => {
    const { subsetLocale } = await loadScript()

    const subset = subsetLocale({ Usage: '用量' }, ['Usage', 'Missing'])

    expect(subset).toEqual({ Usage: '用量' })
  })
})

describe('buildMobileLocales', () => {
  it('writes a trimmed locale file per language from the real mobile sources', async () => {
    const { MOBILE_LOCALE_FILES, buildMobileLocales } = await loadScript()
    const outDir = mkdtempSync(join(tmpdir(), 'mobile-locales-'))

    const result = await buildMobileLocales({ root: process.cwd(), outDir })

    expect(result.literalKeys).toBeGreaterThan(30)
    expect(result.keys).toBeGreaterThanOrEqual(result.literalKeys)

    const english = JSON.parse(
      readFileSync(join(outDir, MOBILE_LOCALE_FILES.en), 'utf8')
    ) as Record<string, string>
    const chinese = JSON.parse(
      readFileSync(join(outDir, MOBILE_LOCALE_FILES.zhCN), 'utf8')
    ) as Record<string, string>
    const traditional = JSON.parse(
      readFileSync(join(outDir, MOBILE_LOCALE_FILES.zhTW), 'utf8')
    ) as Record<string, string>

    // Every language carries the mobile key set, not just the base locale.
    for (const entry of result.report) {
      expect(entry.keys).toBe(result.keys)
    }
    expect(english['Mobile console']).toBe('Mobile console')
    expect(chinese['Mobile console']).toBe('手机控制台')
    expect(traditional['Mobile console']).toBe('手機控制台')

    // The trimmed English bundle must stay orders of magnitude smaller than the
    // shared desktop locale (~700 KB), which is the point of this generator.
    const englishEntry = result.report.find((entry) => entry.code === 'en') as {
      bytes: number
    }
    expect(englishEntry.bytes).toBeLessThan(20_000)
  })
})
