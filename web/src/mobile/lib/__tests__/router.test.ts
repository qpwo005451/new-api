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
import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { parseTab, tabHash, useActiveTab } from '@/mobile/lib/router'

// jsdom fires `hashchange` as a queued task when `location.hash` is written,
// so flush the pending task before asserting on state driven by that write.
const flushHashChange = () => new Promise((resolve) => setTimeout(resolve, 0))

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

describe('useActiveTab', () => {
  beforeEach(async () => {
    window.location.hash = ''
    await flushHashChange()
  })

  it('reads the deep-linked tab on mount', () => {
    window.location.hash = '#/channels'

    const { result } = renderHook(() => useActiveTab())

    expect(result.current[0]).toBe('channels')
  })

  it('follows the hash written after mount', () => {
    const { result } = renderHook(() => useActiveTab())
    expect(result.current[0]).toBe('usage')

    act(() => {
      window.location.hash = '#/routing'
      window.dispatchEvent(new HashChangeEvent('hashchange'))
    })

    expect(result.current[0]).toBe('routing')
  })

  it('removes its hashchange listener on unmount', () => {
    const addListener = vi.spyOn(window, 'addEventListener')
    const removeListener = vi.spyOn(window, 'removeEventListener')
    const { unmount } = renderHook(() => useActiveTab())

    const onHashChange = addListener.mock.calls.find(
      ([type]) => type === 'hashchange'
    )?.[1]
    expect(onHashChange).toBeTypeOf('function')

    unmount()

    expect(removeListener).toHaveBeenCalledWith('hashchange', onHashChange)
  })

  it('writes the selected tab into the hash', async () => {
    const { result } = renderHook(() => useActiveTab())

    await act(async () => {
      result.current[1]('channels')
      await flushHashChange()
    })

    expect(window.location.hash).toBe('#/channels')
    expect(result.current[0]).toBe('channels')
  })
})
