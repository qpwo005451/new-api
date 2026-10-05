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
import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { UsagePage } from '@/mobile/features/usage/components/usage-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

// 2026-10-05T12:00:00Z, pinned so the frozen-window regression test can move
// the clock without depending on the machine timezone.
const PINNED_NOW = new Date('2026-10-05T12:00:00Z')

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

// A fresh element on every render keeps React from bailing out on an identical
// element reference, which is what makes the rerender in the frozen-window
// test actually re-run `UsagePage`.
function pageElement() {
  return (
    <QueryClientProvider client={mobileQueryClient}>
      <UsagePage />
    </QueryClientProvider>
  )
}

describe('UsagePage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows totals from the quota data aggregate', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.startsWith('/api/log/self/stat')) {
          return jsonResponse({
            success: true,
            data: { quota: 1650, rpm: 3, tpm: 400 },
          })
        }
        if (url.startsWith('/api/data/self')) {
          return jsonResponse({
            success: true,
            data: [
              {
                model_name: 'gpt-5',
                created_at: 1,
                count: 16,
                quota: 1650,
                token_used: 1530,
              },
            ],
          })
        }
        return jsonResponse({ success: true, data: { items: [], total: 0 } })
      })
    )

    render(pageElement())

    await waitFor(() => expect(screen.getByText('16')).toBeInTheDocument())
    // formatTokens shortens thousands, and both the KPI card and the model row
    // render 1530 tokens, so more than one matching node is expected.
    expect(screen.getAllByText('1.5K').length).toBeGreaterThan(0)
    expect(screen.getByText('gpt-5')).toBeInTheDocument()
    expect(
      screen.getByText('Summary data updates every few minutes.')
    ).toBeInTheDocument()
  })

  it('switches the scope to the whole instance', async () => {
    const fetchMock = vi.fn(async (_url: string) =>
      jsonResponse({ success: true, data: { quota: 0, rpm: 0, tpm: 0 } })
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    render(pageElement())
    await user.click(screen.getByRole('button', { name: 'All' }))

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([url]) =>
          String(url).startsWith('/api/log/stat')
        )
      ).toBe(true)
    )
  })

  it('renders the current rate when the stat endpoint answers an object', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.startsWith('/api/log/self/stat')) {
          return jsonResponse({
            success: true,
            data: { quota: 0, rpm: 5, tpm: 300 },
          })
        }
        return jsonResponse({ success: true, data: [] })
      })
    )

    render(pageElement())

    await waitFor(() =>
      expect(
        screen.getByText('Nothing to show for this range.')
      ).toBeInTheDocument()
    )
    // The KPI must render real numbers instead of the `—` fallback when the
    // stat payload has the documented shape.
    await waitFor(() => expect(screen.getByText('5 / 300')).toBeInTheDocument())
    expect(screen.queryByText('—')).not.toBeInTheDocument()
  })

  it('shows the error state when the recent log request fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.startsWith('/api/log/self/stat')) {
          return jsonResponse({
            success: true,
            data: { quota: 0, rpm: 1, tpm: 2 },
          })
        }
        if (url.startsWith('/api/data/self')) {
          return jsonResponse({
            success: true,
            data: [
              {
                model_name: 'gpt-5',
                created_at: 1,
                count: 1,
                quota: 1,
                token_used: 2,
              },
            ],
          })
        }
        if (url.startsWith('/api/log/self')) {
          return jsonResponse({ success: false, message: 'boom' }, 500)
        }
        return jsonResponse({ success: true, data: { items: [], total: 0 } })
      })
    )

    render(pageElement())

    await waitFor(() => expect(screen.getByRole('alert')).toBeInTheDocument())
    expect(screen.getByText('Load failed')).toBeInTheDocument()
    expect(screen.queryByText('No recent requests')).not.toBeInTheDocument()
  })

  it('keeps the time window frozen when a response arrives a second later', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.setSystemTime(PINNED_NOW)
    const fetchMock = vi.fn(async (url: string) => {
      if (url.startsWith('/api/log/self/stat')) {
        return jsonResponse({
          success: true,
          data: { quota: 1650, rpm: 3, tpm: 400 },
        })
      }
      if (url.startsWith('/api/data/self')) {
        return jsonResponse({
          success: true,
          data: [
            {
              model_name: 'gpt-5',
              created_at: 1,
              count: 16,
              quota: 1650,
              token_used: 1530,
            },
          ],
        })
      }
      return jsonResponse({ success: true, data: { items: [], total: 0 } })
    })
    vi.stubGlobal('fetch', fetchMock)

    const { rerender } = render(pageElement())
    await waitFor(() => expect(screen.getByText('16')).toBeInTheDocument())

    const aggregateRequests = () =>
      fetchMock.mock.calls
        .map(([url]) => String(url))
        .filter((url) => url.startsWith('/api/data/self'))
    expect(aggregateRequests()).toHaveLength(1)
    const firstRequest = aggregateRequests()[0]

    // A slow response lands more than a second after mount and re-renders the
    // page. With `Date.now()` read during render this would mint a new query
    // key, refetch every range query and blank the panel again; the frozen
    // anchor must keep the request identical.
    vi.setSystemTime(new Date('2026-10-05T12:00:05Z'))
    rerender(pageElement())
    await new Promise((resolve) => setTimeout(resolve, 20))

    expect(aggregateRequests()).toEqual([firstRequest])
    expect(firstRequest).toContain(
      `end_timestamp=${Math.floor(PINNED_NOW.getTime() / 1000)}`
    )
  })
})
