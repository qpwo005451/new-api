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
import { readFileSync } from 'node:fs'
import path from 'node:path'
import vm from 'node:vm'

import { afterEach, describe, expect, it } from 'vitest'

// The first-paint theme is applied by an inline script in the HTML head, before
// any module loads. Executing that exact script here keeps it honest: if it
// drifts from ThemeProvider's initial resolution, these cases fail instead of
// the flashed theme only showing up in a real browser.
const html = readFileSync(path.resolve('src/mobile/index.html'), 'utf8')
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')

function extractBootstrap(): string {
  const match = html.match(/<script>([\s\S]*?)<\/script>/)
  if (!match) {
    throw new Error('index.html is missing the inline theme bootstrap script')
  }
  return match[1]
}

function runBootstrap(mode: string | null, prefersDark: boolean): string {
  window.localStorage.clear()
  if (mode !== null) {
    window.localStorage.setItem('newapi:theme:v1:mode', mode)
  }
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: (query: string): MediaQueryList => ({
      matches:
        prefersDark && query.includes('prefers-color-scheme: dark')
          ? true
          : query.includes('prefers-reduced-motion') &&
            !query.includes('no-preference'),
      media: query,
      onchange: null,
      addListener: () => undefined,
      removeListener: () => undefined,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      dispatchEvent: () => false,
    }),
  })
  document.documentElement.classList.remove('light', 'dark')

  vm.runInThisContext(extractBootstrap())

  return document.documentElement.classList.contains('dark') ? 'dark' : 'light'
}

afterEach(() => {
  if (originalMatchMedia) {
    Object.defineProperty(window, 'matchMedia', originalMatchMedia)
  }
  document.documentElement.classList.remove('light', 'dark')
})

describe('mobile index.html theme bootstrap', () => {
  it('runs an inline head script before the app root', () => {
    expect(extractBootstrap()).not.toContain('import')
    const scriptIndex = html.indexOf('newapi:theme:v1:mode')
    const rootIndex = html.indexOf('id="root"')
    const headEnd = html.indexOf('</head>')

    expect(scriptIndex).toBeGreaterThan(-1)
    expect(headEnd).toBeGreaterThan(scriptIndex)
    expect(rootIndex).toBeGreaterThan(scriptIndex)
  })

  it('honours a stored explicit mode over the system preference', () => {
    expect(runBootstrap('dark', false)).toBe('dark')
    expect(runBootstrap('light', true)).toBe('light')
  })

  it('falls back to the system preference for system or unknown modes', () => {
    expect(runBootstrap('system', true)).toBe('dark')
    expect(runBootstrap('system', false)).toBe('light')
    expect(runBootstrap(null, true)).toBe('dark')
    expect(runBootstrap('sepia', false)).toBe('light')
  })
})
