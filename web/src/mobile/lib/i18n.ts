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

// Each locale file holds every desktop string, so locales are only ever loaded
// as separate async chunks: the console ships no locale data in its first screen.
const localeLoaders = {
  en: () => import('@/i18n/locales/en.json'),
  zhCN: () => import('@/i18n/locales/zh.json'),
  zhTW: () => import('@/i18n/locales/zh-TW.json'),
  fr: () => import('@/i18n/locales/fr.json'),
  ru: () => import('@/i18n/locales/ru.json'),
  ja: () => import('@/i18n/locales/ja.json'),
  vi: () => import('@/i18n/locales/vi.json'),
} as const

type MobileLocaleCode = keyof typeof localeLoaders

const mobileLocaleCodes = Object.keys(localeLoaders) as MobileLocaleCode[]

function isMobileLocaleCode(value: string): value is MobileLocaleCode {
  return Object.hasOwn(localeLoaders, value)
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

async function loadLocale(code: MobileLocaleCode): Promise<void> {
  const [bundle, fallback] = await Promise.all([
    localeLoaders[code](),
    code === 'en' ? undefined : localeLoaders.en(),
  ])

  // A locale file is its own namespace map (`{ translation: { ... } }`), the
  // same shape `@/i18n/config` feeds to i18next's `resources` option.
  for (const [namespace, strings] of Object.entries(bundle.default)) {
    i18n.addResourceBundle(code, namespace, strings, true, true)
  }
  if (fallback) {
    for (const [namespace, strings] of Object.entries(fallback.default)) {
      i18n.addResourceBundle('en', namespace, strings, true, true)
    }
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
