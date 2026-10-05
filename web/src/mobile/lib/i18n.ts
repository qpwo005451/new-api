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
import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'

import { convertDetectedLanguage } from '@/i18n/languages'

// Each locale file holds every desktop string (~7k keys), so the console
// fetches its own trimmed bundles from `/m/locales` instead: they are generated
// from the keys these sources reference (`scripts/build-mobile-locales.mjs`) and
// copied into the mobile build by `public-mobile`. No locale data is bundled
// into the console's JavaScript, and only the active language is downloaded.
const MOBILE_LOCALE_FILES = {
  en: 'en.json',
  zhCN: 'zh.json',
  zhTW: 'zh-TW.json',
  fr: 'fr.json',
  ru: 'ru.json',
  ja: 'ja.json',
  vi: 'vi.json',
} as const

type MobileLocaleCode = keyof typeof MOBILE_LOCALE_FILES

const mobileLocaleCodes = Object.keys(MOBILE_LOCALE_FILES) as MobileLocaleCode[]

function isMobileLocaleCode(value: string): value is MobileLocaleCode {
  return Object.hasOwn(MOBILE_LOCALE_FILES, value)
}

// The shared `i18nextLng` cache the desktop writes already holds interface codes
// (`zhTW`), so an exact match has to win before the BCP-47 mapping runs:
// `convertDetectedLanguage('zhTW')` reads that code as a plain `zh` tag and
// would downgrade it to `zhCN`. Only then reuse the project's browser-tag
// mapping (`zh` -> `zhCN`, `zh-Hant-TW` -> `zhTW`) and narrow region tags onto
// their primary code (`fr-FR` -> `fr`).
export function resolveMobileLocale(value: string): MobileLocaleCode {
  const trimmed = value.trim()
  if (isMobileLocaleCode(trimmed)) return trimmed

  const converted = convertDetectedLanguage(trimmed.toLowerCase())
  if (isMobileLocaleCode(converted)) return converted

  const primary = converted.split('-')[0]
  return isMobileLocaleCode(primary) ? primary : 'en'
}

function detectedLocaleValue(): string {
  const stored = localStorage.getItem('i18nextLng')
  return stored ?? navigator.language
}

/**
 * Fetches one trimmed locale bundle. A failed fetch is not fatal: the console
 * then renders the English source keys, which is also what i18next falls back
 * to for any key a bundle does not carry.
 */
async function fetchLocaleStrings(
  code: MobileLocaleCode
): Promise<Record<string, string> | null> {
  try {
    const response = await fetch(`/m/locales/${MOBILE_LOCALE_FILES[code]}`)
    if (!response.ok) return null
    return (await response.json()) as Record<string, string>
  } catch {
    return null
  }
}

async function loadLocale(code: MobileLocaleCode): Promise<void> {
  const [strings, fallback] = await Promise.all([
    fetchLocaleStrings(code),
    code === 'en' ? Promise.resolve(null) : fetchLocaleStrings('en'),
  ])

  if (strings) {
    i18n.addResourceBundle(code, 'translation', strings, true, true)
  }
  if (fallback) {
    i18n.addResourceBundle('en', 'translation', fallback, true, true)
  }
  await i18n.changeLanguage(code)
}

export async function initializeMobileI18n(): Promise<void> {
  const code = resolveMobileLocale(detectedLocaleValue())

  await i18n.use(initReactI18next).init({
    resources: {},
    lng: code,
    fallbackLng: 'en',
    supportedLngs: mobileLocaleCodes,
    load: 'currentOnly',
    nsSeparator: false, // Allow literal colons in keys (e.g. URLs, labels)
    debug: import.meta.env.DEV,
    interpolation: { escapeValue: false },
  })

  localStorage.setItem('i18nextLng', code)
  await loadLocale(code)
}
