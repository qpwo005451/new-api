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
import { describe, expect, it } from 'vitest'

import { parseTab, tabHash } from '@/mobile/lib/router'

describe('parseTab', () => {
  it('returns usage for an empty hash', () => {
    expect(parseTab('')).toBe('usage')
  })

  it('parses each known tab', () => {
    expect(parseTab('#/models')).toBe('models')
    expect(parseTab('#/routing')).toBe('routing')
    expect(parseTab('#/channels')).toBe('channels')
  })

  it('falls back to usage for an unknown hash', () => {
    expect(parseTab('#/nope')).toBe('usage')
    expect(parseTab('#/usage/extra')).toBe('usage')
  })

  it('ignores a trailing slash', () => {
    expect(parseTab('#/routing/')).toBe('routing')
  })
})

describe('tabHash', () => {
  it('builds the hash for a tab', () => {
    expect(tabHash('channels')).toBe('#/channels')
    expect(tabHash('usage')).toBe('#/usage')
  })
})
