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
import { beforeEach, describe, expect, it } from 'vitest'

import { initializeMobileI18n, resolveMobileLocale } from '@/mobile/lib/i18n'

describe('resolveMobileLocale', () => {
  it('maps browser tags onto the interface language codes', () => {
    expect(resolveMobileLocale('zh')).toBe('zhCN')
    expect(resolveMobileLocale('zh-CN')).toBe('zhCN')
    expect(resolveMobileLocale('zh-Hant-TW')).toBe('zhTW')
    expect(resolveMobileLocale('fr-FR')).toBe('fr')
    expect(resolveMobileLocale('de-DE')).toBe('en')
  })
})

describe('initializeMobileI18n', () => {
  beforeEach(async () => {
    localStorage.clear()
    await i18n.changeLanguage('en')
  })

  it('loads only the active locale bundle', async () => {
    localStorage.setItem('i18nextLng', 'zh')

    await initializeMobileI18n()

    expect(i18n.resolvedLanguage).toBe('zhCN')
    expect(i18n.hasResourceBundle('zhCN', 'translation')).toBe(true)
    expect(i18n.hasResourceBundle('en', 'translation')).toBe(true)
    expect(i18n.hasResourceBundle('ja', 'translation')).toBe(false)
    expect(i18n.t('Usage')).not.toBe('Usage')
  })
})
