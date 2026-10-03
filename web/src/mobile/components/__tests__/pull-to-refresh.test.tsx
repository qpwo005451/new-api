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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { PullToRefresh } from '@/mobile/components/pull-to-refresh'

function touchStart(element: HTMLElement, clientY: number) {
  fireEvent.touchStart(element, { touches: [{ clientY }] })
}

function renderScroller(scrollTop: number, onRefresh: () => Promise<void>) {
  const { container } = render(
    <PullToRefresh onRefresh={onRefresh}>
      <p>content</p>
    </PullToRefresh>
  )
  const scroller = container.firstElementChild as HTMLElement
  Object.defineProperty(scroller, 'scrollTop', {
    value: scrollTop,
    configurable: true,
  })
  return scroller
}

describe('PullToRefresh', () => {
  it('refreshes after a long pull from the top', async () => {
    const onRefresh = vi.fn().mockResolvedValue(undefined)
    const scroller = renderScroller(0, onRefresh)

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchEnd(scroller)

    await waitFor(() => expect(onRefresh).toHaveBeenCalledTimes(1))
  })

  it('ignores a short pull', async () => {
    const onRefresh = vi.fn().mockResolvedValue(undefined)
    const scroller = renderScroller(0, onRefresh)

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 20 }] })
    fireEvent.touchEnd(scroller)

    await waitFor(() =>
      expect(scroller).toHaveAttribute('data-refreshing', 'false')
    )
    expect(onRefresh).not.toHaveBeenCalled()
  })

  it('ignores a pull that starts away from the top', () => {
    const onRefresh = vi.fn().mockResolvedValue(undefined)
    const scroller = renderScroller(200, onRefresh)

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchEnd(scroller)

    expect(onRefresh).not.toHaveBeenCalled()
  })

  it('does not start a second refresh while one is in flight', async () => {
    let resolveRefresh: () => void = () => undefined
    const onRefresh = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveRefresh = resolve
        })
    )
    const scroller = renderScroller(0, onRefresh)

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchEnd(scroller)
    await waitFor(() => expect(onRefresh).toHaveBeenCalledTimes(1))
    await waitFor(() =>
      expect(scroller).toHaveAttribute('data-refreshing', 'true')
    )

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchEnd(scroller)
    expect(onRefresh).toHaveBeenCalledTimes(1)

    resolveRefresh()
    await waitFor(() =>
      expect(scroller).toHaveAttribute('data-refreshing', 'false')
    )
    expect(onRefresh).toHaveBeenCalledTimes(1)
  })

  it('clears the refreshing state when the refresh fails', async () => {
    const onRefresh = vi.fn().mockRejectedValue(new Error('offline'))
    const scroller = renderScroller(0, onRefresh)

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchEnd(scroller)

    await waitFor(() =>
      expect(scroller).toHaveAttribute('data-refreshing', 'false')
    )
    expect(onRefresh).toHaveBeenCalledTimes(1)
  })

  it('cancels an armed pull when the touch is cancelled', async () => {
    const onRefresh = vi.fn().mockResolvedValue(undefined)
    const scroller = renderScroller(0, onRefresh)

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchCancel(scroller)
    // A cancelled gesture can still be followed by a touchEnd in the browser;
    // the cancel must clear the armed threshold so that end is a no-op.
    fireEvent.touchEnd(scroller)

    await waitFor(() =>
      expect(scroller).toHaveAttribute('data-refreshing', 'false')
    )
    expect(onRefresh).not.toHaveBeenCalled()
  })

  it('announces the refreshing state to assistive technology', async () => {
    let resolveRefresh: () => void = () => undefined
    const onRefresh = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveRefresh = resolve
        })
    )
    const scroller = renderScroller(0, onRefresh)

    const status = screen.getByRole('status')
    expect(status).toHaveAttribute('aria-live', 'polite')
    expect(status).toBeEmptyDOMElement()

    touchStart(scroller, 0)
    fireEvent.touchMove(scroller, { touches: [{ clientY: 120 }] })
    fireEvent.touchEnd(scroller)

    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent('Refreshing...')
    )

    resolveRefresh()
    await waitFor(() =>
      expect(screen.getByRole('status')).toBeEmptyDOMElement()
    )
  })
})
