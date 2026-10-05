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
import { render, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { STATUS_QUERY_KEY } from '@/lib/status-query'
import { mobileQueryClient } from '@/mobile/lib/query-client'
import { MobileProviders } from '@/mobile/providers'

// The shared test setup installs a matchMedia stub that only answers
// prefers-reduced-motion; this suite replaces it to drive the color scheme and
// restores the original descriptor afterwards.
const originalMatchMedia = Object.getOwnPropertyDescriptor(window, 'matchMedia')

function stubSystemColorScheme(prefersDark: boolean) {
  Object.defineProperty(window, 'matchMedia', {
    configurable: true,
    value: (query: string): MediaQueryList => ({
      matches: query.includes('prefers-color-scheme: dark')
        ? prefersDark
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
}

describe('MobileProviders theme', () => {
  beforeEach(() => {
    window.localStorage.clear()
    mobileQueryClient.clear()
    mobileQueryClient.setQueryData(STATUS_QUERY_KEY, {})
    document.documentElement.classList.remove('light', 'dark')
  })

  afterEach(() => {
    if (originalMatchMedia) {
      Object.defineProperty(window, 'matchMedia', originalMatchMedia)
    }
    document.documentElement.classList.remove('light', 'dark')
  })

  it('applies the dark class when the system prefers dark', async () => {
    stubSystemColorScheme(true)

    render(<MobileProviders>{null}</MobileProviders>)

    await waitFor(() =>
      expect(document.documentElement.classList.contains('dark')).toBe(true)
    )
  })

  it('applies the light class when the system prefers light', async () => {
    stubSystemColorScheme(false)

    render(<MobileProviders>{null}</MobileProviders>)

    await waitFor(() =>
      expect(document.documentElement.classList.contains('light')).toBe(true)
    )
  })
})
