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
import { beforeEach, describe, expect, it } from 'vitest'

import { clearPat, isPlausiblePat, readPat, writePat } from '@/mobile/lib/pat-store'

describe('pat-store', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('returns an empty string when no token was stored', () => {
    expect(readPat()).toBe('')
  })

  it('round-trips a token and trims surrounding whitespace', () => {
    writePat('  abcdefghijklmnopqrstuvwxyz012  ')

    expect(readPat()).toBe('abcdefghijklmnopqrstuvwxyz012')
  })

  it('clears a stored token', () => {
    writePat('abcdefghijklmnopqrstuvwxyz012')
    clearPat()

    expect(readPat()).toBe('')
  })

  it('accepts tokens of the 29 to 32 characters produced by the server', () => {
    expect(isPlausiblePat('a'.repeat(29))).toBe(true)
    expect(isPlausiblePat('a'.repeat(32))).toBe(true)
  })

  it('rejects tokens that are too short, too long, or contain spaces', () => {
    expect(isPlausiblePat('')).toBe(false)
    expect(isPlausiblePat('a'.repeat(28))).toBe(false)
    expect(isPlausiblePat('a'.repeat(33))).toBe(false)
    expect(isPlausiblePat('abcdefghijklmnopqrstuvwxy z012')).toBe(false)
  })

  it('falls back to an empty string when storage is unavailable', () => {
    const original = window.localStorage.getItem
    Object.defineProperty(window, 'localStorage', {
      configurable: true,
      value: {
        getItem: () => {
          throw new Error('storage disabled')
        },
      },
    })

    expect(readPat()).toBe('')
    Object.defineProperty(window, 'localStorage', { configurable: true, value: { getItem: original } })
  })
})
