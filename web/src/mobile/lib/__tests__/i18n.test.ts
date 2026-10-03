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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { initializeMobileI18n, resolveMobileLocale } from '@/mobile/lib/i18n'

describe('resolveMobileLocale', () => {
  it('maps browser tags onto the interface language codes', () => {
    expect(resolveMobileLocale('zh')).toBe('zhCN')
    expect(resolveMobileLocale('zh-CN')).toBe('zhCN')
    expect(resolveMobileLocale('zh-Hant-TW')).toBe('zhTW')
    expect(resolveMobileLocale('fr-FR')).toBe('fr')
    expect(resolveMobileLocale('de-DE')).toBe('en')
  })

  it('keeps interface language codes the desktop already cached', () => {
    expect(resolveMobileLocale('zhTW')).toBe('zhTW')
    expect(resolveMobileLocale('zhCN')).toBe('zhCN')
    expect(resolveMobileLocale('ja')).toBe('ja')
  })
})

describe('initializeMobileI18n', () => {
  const localePayload: Record<string, Record<string, string>> = {
    'en.json': { Usage: 'Usage', 'Mobile console': 'Mobile console' },
    'zh.json': { Usage: '用量', 'Mobile console': '手机控制台' },
    'zh-TW.json': { Usage: '用量', 'Mobile console': '手機控制台' },
  }

  function stubLocaleFetch() {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const file = String(input).split('/').pop() as string
      const body = localePayload[file]
      if (!body) return new Response('', { status: 404 })
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })
    vi.stubGlobal('fetch', fetchMock)
    return fetchMock
  }

  beforeEach(async () => {
    localStorage.clear()
    vi.unstubAllGlobals()
    await i18n.changeLanguage('en')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('fetches the trimmed locale bundle for the resolved language', async () => {
    localStorage.setItem('i18nextLng', 'zh')
    const fetchMock = stubLocaleFetch()

    await initializeMobileI18n()

    expect(fetchMock).toHaveBeenCalledWith('/m/locales/zh.json')
    expect(i18n.resolvedLanguage).toBe('zhCN')
    expect(i18n.t('Usage')).toBe('用量')
    expect(i18n.t('Mobile console')).toBe('手机控制台')
    expect(fetchMock).not.toHaveBeenCalledWith('/m/locales/ja.json')
  })

  it('keeps English as the fallback language for untranslated keys', async () => {
    localStorage.setItem('i18nextLng', 'zhTW')
    const fetchMock = stubLocaleFetch()

    await initializeMobileI18n()

    expect(fetchMock).toHaveBeenCalledWith('/m/locales/zh-TW.json')
    expect(fetchMock).toHaveBeenCalledWith('/m/locales/en.json')
    expect(i18n.t('Mobile console')).toBe('手機控制台')
    // Nothing in the trimmed bundle is missing, so the key itself is the last
    // line of defence for strings the console adds later.
    expect(i18n.t('Not translated anywhere')).toBe('Not translated anywhere')
  })

  it('renders the English source keys when the locale file cannot be fetched', async () => {
    localStorage.setItem('i18nextLng', 'zh')
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new Error('network down')
      })
    )

    await expect(initializeMobileI18n()).resolves.toBeUndefined()

    // i18next only sets `resolvedLanguage` once a language holds a translation,
    // so a missing bundle leaves the requested language in place and renders
    // the English source keys.
    expect(i18n.language).toBe('zhCN')
    expect(i18n.t('Usage')).toBe('Usage')
  })

  it('keeps a cached Traditional Chinese preference across reloads', async () => {
    localStorage.setItem('i18nextLng', 'zhTW')
    stubLocaleFetch()

    await initializeMobileI18n()
    await initializeMobileI18n()

    expect(i18n.resolvedLanguage).toBe('zhTW')
    expect(localStorage.getItem('i18nextLng')).toBe('zhTW')
  })
})
