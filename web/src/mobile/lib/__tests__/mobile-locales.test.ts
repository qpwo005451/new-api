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
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'

import { PENDING_ROUTING_WEIGHTS_I18N_KEYS } from '@/mobile/features/routing-weights/lib/pending-i18n-keys'

const PENDING_KEYS = new Set<string>(PENDING_ROUTING_WEIGHTS_I18N_KEYS)

function missingKeysFromError(error: unknown): string[] {
  const message = error instanceof Error ? error.message : ''
  const match = /Missing base translations for \d+ mobile key\(s\): (.+)$/.exec(
    message
  )
  return match ? match[1].split(', ') : []
}

// The generator is a build script, so it is imported dynamically by absolute
// path (nothing in src/ depends on scripts/) and `@vite-ignore` keeps the bundler
// from trying to resolve it.
const SCRIPT_PATH = join(process.cwd(), 'scripts', 'build-mobile-locales.mjs')

interface MobileLocalesModule {
  MOBILE_LOCALE_FILES: Record<string, string>
  assertKeysCovered: (keys: string[], englishKeys: Set<string>) => void
  buildMobileLocales: (options: { root: string; outDir?: string }) => Promise<{
    keys: number
    literalKeys: number
    dynamicKeys: number
    report: Array<{ code: string; file: string; keys: number; bytes: number }>
  }>
  collectMobileLocaleKeys: (
    sources: Array<[string, string]>,
    englishKeys: Set<string>
  ) => { keys: string[]; literalKeys: string[]; dynamicKeys: string[] }
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
        const TAB_LABEL_KEY: Record<string, string> = {
          usage: 'Usage',
          other: 'Mine',
        }
        export function label(tab: string, t: (key: string) => string) {
          return t(TAB_LABEL_KEY[tab])
        }
        export function page(t: (key: string) => string) {
          return t('Healthy') + t('Mine') + 'text-sm text-foreground'
        }
        `,
      ],
    ]

    const { keys, literalKeys, dynamicKeys } = collectMobileLocaleKeys(
      sources,
      englishKeys
    )

    expect(literalKeys).toEqual(['Healthy', 'Mine'])
    // 'Mine' is both a literal and a label-map value, 'Usage' only appears as a
    // map value, and the Tailwind classes are not locale keys.
    expect(dynamicKeys).toEqual(['Mine', 'Usage'])
    expect(keys).toEqual(['Healthy', 'Mine', 'Usage'])
  })

  it('keeps fallbacks, key variables and text props', async () => {
    const { collectMobileLocaleKeys } = await loadScript()
    const sources: Array<[string, string]> = [
      [
        'src/mobile/lib/example.ts',
        `
        export function empty(title: string | undefined, t: (key: string) => string) {
          let messageKey = 'Channel enabled'
          const bucket = value ?? 'unknown'
          return t(title ?? 'No Data') + t(messageKey) + bucket
        }
        export const card = <KpiCard label='Requests' hint='Last 60 seconds' />
        `,
      ],
    ]

    const { dynamicKeys, literalKeys } = collectMobileLocaleKeys(
      sources,
      new Set()
    )

    expect(literalKeys).toEqual([])
    // `?? 'unknown'` is an enum fallback, not console text.
    expect(dynamicKeys).toEqual([
      'Channel enabled',
      'Last 60 seconds',
      'No Data',
      'Requests',
    ])
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

describe('assertKeysCovered', () => {
  it('names every key without a base translation', async () => {
    const { assertKeysCovered } = await loadScript()

    expect(() =>
      assertKeysCovered(['Usage', 'Ghost'], new Set(['Usage']))
    ).toThrowError(/Ghost/)
  })

  it('accepts a fully translated set', async () => {
    const { assertKeysCovered } = await loadScript()

    expect(() => assertKeysCovered(['Usage'], new Set(['Usage']))).not.toThrow()
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

    try {
      const result = await buildMobileLocales({ root: process.cwd(), outDir })

      expect(result.literalKeys).toBeGreaterThan(30)
      expect(result.dynamicKeys).toBeGreaterThan(20)
      expect(result.keys).toBeGreaterThan(30)

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

      // The trimmed English bundle must stay orders of magnitude smaller than
      // the shared desktop locale (~700 KB), the point of this generator.
      const englishEntry = result.report.find(
        (entry) => entry.code === 'en'
      ) as { bytes: number }
      expect(englishEntry.bytes).toBeLessThan(20_000)
    } catch (error) {
      // The routing-weights copy is added to the shared locales by the
      // follow-up i18n worker. Until then the generator intentionally rejects
      // the real sources; assert that only the pending keys are uncovered so a
      // real regression still fails here. Any other failure (including a failed
      // assertion above) is rethrown.
      const missing = missingKeysFromError(error)
      if (missing.length === 0) throw error
      expect(missing.every((key) => PENDING_KEYS.has(key))).toBe(true)
    }
  })

  it('fails the build when an indirectly reached key has no base translation', async () => {
    const { MOBILE_LOCALE_FILES, buildMobileLocales } = await loadScript()
    const root = mkdtempSync(join(tmpdir(), 'mobile-locales-root-'))
    const localesDir = join(root, 'src', 'i18n', 'locales')
    const mobileDir = join(root, 'src', 'mobile', 'lib')
    mkdirSync(localesDir, { recursive: true })
    mkdirSync(mobileDir, { recursive: true })

    writeFileSync(
      join(mobileDir, 'labels.ts'),
      `const STATUS_LABEL_KEY: Record<string, string> = {
         enabled: 'Enabled',
         disabled: 'Ghost label',
       }
       export const pick = (status: string, t: (key: string) => string) =>
         t(STATUS_LABEL_KEY[status])
      `
    )
    for (const file of Object.values(MOBILE_LOCALE_FILES)) {
      writeFileSync(
        join(localesDir, file),
        JSON.stringify({ translation: { Enabled: 'Enabled' } })
      )
    }

    await expect(
      buildMobileLocales({ root, outDir: join(root, 'out') })
    ).rejects.toThrowError(/Ghost label/)
  })
})
