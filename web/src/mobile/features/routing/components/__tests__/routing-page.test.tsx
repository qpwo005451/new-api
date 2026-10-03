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
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { RoutingPage } from '@/mobile/features/routing/components/routing-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

// 2026-10-05T12:00:00Z, pinned so the preset-switch test can move the clock
// deterministically without depending on the machine timezone.
const PINNED_NOW = new Date('2026-10-05T12:00:00Z')

const routingStats = {
  requests: 400,
  errors: 0,
  scanned: 10,
  truncated: false,
  switched: 7,
  switched_success: 6,
  switched_failed: 1,
  sticky: 12,
  by_model_channel: [
    {
      model_name: 'gpt-5',
      channel_id: 1,
      channel_name: 'a',
      requests: 100,
      errors: 0,
      avg_use_time: 1.5,
      priority: 0,
      weight: 10,
    },
    {
      model_name: 'gpt-5',
      channel_id: 2,
      channel_name: 'b',
      requests: 300,
      errors: 0,
      avg_use_time: 1.1,
      priority: 0,
      weight: 30,
    },
  ],
  window: { start: 0, end: 3600, bucket_seconds: 600 },
  trend: [{ timestamp: 0, requests: 400, switched: 4 }],
  switches: [{ from: 1, to: 2, count: 4 }],
  switch_reasons: [{ reason: 'channel_error', count: 4 }],
  affinity_by_rule: [
    { rule_name: 'default', sticky_requests: 12, distinct_keys: 3 },
  ],
}

function renderPage() {
  return render(
    <QueryClientProvider client={mobileQueryClient}>
      <RoutingPage />
    </QueryClientProvider>
  )
}

// The label text is shared by the KPI card and the matching section heading, so
// scope the lookup to the card (`data-slot='card'`) before asserting the value.
function kpiCard(label: string): HTMLElement {
  const card = screen
    .getAllByText(label)
    .map((node) => node.closest('[data-slot="card"]'))
    .find((node): node is HTMLElement => node instanceof HTMLElement)
  if (!card) {
    throw new Error(`No KPI card found for "${label}"`)
  }
  return card
}

describe('RoutingPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows per-model request split with configured weights', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).startsWith('/api/log/routing_stats')) {
          return jsonResponse({ success: true, data: routingStats })
        }
        return jsonResponse({ success: true, data: { entries: [], total: 0 } })
      })
    )

    renderPage()

    await waitFor(() => expect(screen.getByText('gpt-5')).toBeInTheDocument())

    // The KPI reports the server total (7), not the sum of the truncated
    // top-5 pair list (4); a regression back to summing the pairs fails here.
    expect(
      within(kpiCard('Switched requests')).getByText('7')
    ).toBeInTheDocument()
    expect(within(kpiCard('Requests')).getByText('400')).toBeInTheDocument()
    // The pair list stays the source for the switches section rows.
    expect(screen.getByText('1 → 2')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
  })

  it('discloses a truncated switch scan', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).startsWith('/api/log/routing_stats')) {
          return jsonResponse({
            success: true,
            data: { ...routingStats, scanned: 10, truncated: true },
          })
        }
        return jsonResponse({ success: true, data: { entries: [], total: 0 } })
      })
    )

    renderPage()

    await waitFor(() =>
      expect(screen.getByRole('status')).toHaveTextContent(
        'Some switches in this window were not scanned.'
      )
    )
  })

  it('falls back to the channel id when the channel name is blank', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).startsWith('/api/log/routing_stats')) {
          return jsonResponse({
            success: true,
            data: {
              ...routingStats,
              by_model_channel: [
                {
                  model_name: 'gpt-5',
                  channel_id: 9,
                  channel_name: '',
                  requests: 3,
                  errors: 0,
                  avg_use_time: 1,
                  priority: 0,
                  weight: 1,
                },
              ],
            },
          })
        }
        return jsonResponse({ success: true, data: { entries: [], total: 0 } })
      })
    )

    renderPage()

    await waitFor(() => expect(screen.getByText('#9')).toBeInTheDocument())
  })

  it('refetches once with a new window when the time preset changes', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.setSystemTime(PINNED_NOW)
    // Hold the affinity response back so its arrival can be delivered on a
    // later wall-clock second, forcing one extra render after the switch.
    let resolveAffinity!: (response: Response) => void
    const pendingAffinity = new Promise<Response>((resolve) => {
      resolveAffinity = resolve
    })
    const fetchMock = vi.fn(async (url: string) => {
      if (String(url).startsWith('/api/log/routing_stats')) {
        return jsonResponse({ success: true, data: routingStats })
      }
      if (String(url).startsWith('/api/log/channel_affinity_bindings')) {
        return pendingAffinity
      }
      return jsonResponse({ success: true, data: { entries: [], total: 0 } })
    })
    vi.stubGlobal('fetch', fetchMock)

    renderPage()
    await waitFor(() => expect(screen.getByText('gpt-5')).toBeInTheDocument())

    const routingRequests = () =>
      fetchMock.mock.calls
        .map(([url]) => String(url))
        .filter((url) => url.startsWith('/api/log/routing_stats'))
    expect(routingRequests()).toHaveLength(1)
    const [firstUrl] = routingRequests()

    // The frozen-window regression (Task 8) would mint new query keys on every
    // render and refetch; the URL set must stay exactly one per preset.
    vi.setSystemTime(new Date('2026-10-05T12:00:30Z'))
    fireEvent.click(screen.getByRole('button', { name: 'Last 7 days' }))
    await waitFor(() => expect(routingRequests()).toHaveLength(2))

    const requests = routingRequests()
    expect(requests).toHaveLength(2)
    const first = new URL(firstUrl, 'http://localhost')
    const second = new URL(requests[1], 'http://localhost')
    expect(second.searchParams.get('start_timestamp')).not.toBe(
      first.searchParams.get('start_timestamp')
    )
    expect(second.searchParams.get('end_timestamp')).not.toBe(
      first.searchParams.get('end_timestamp')
    )

    // Deliver the affinity response on a later second than the preset switch.
    // Resolving it re-renders the page; if the window anchor were recomputed
    // during render, that render would change the range and mint a third
    // routing_stats request. The count staying at two is the real guard, not
    // the immediate fetch that the preset switch itself already proves.
    vi.setSystemTime(new Date('2026-10-05T12:00:33Z'))
    resolveAffinity(
      jsonResponse({ success: true, data: { entries: [], total: 4 } })
    )
    // Wait until the late render actually landed: the sticky key count now
    // comes from the resolved affinity response, so the page re-rendered.
    await waitFor(() => expect(screen.getByText('4 keys')).toBeInTheDocument())
    // That late render must not have moved the window and minted a third request.
    expect(routingRequests()).toHaveLength(2)
    // The affinity endpoint does not key on the range, so it must not refetch.
    expect(
      fetchMock.mock.calls.filter(([url]) =>
        String(url).startsWith('/api/log/channel_affinity_bindings')
      )
    ).toHaveLength(1)
  })

  it('shows an empty state when the window has no routing rows', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).startsWith('/api/log/routing_stats')) {
          return jsonResponse({
            success: true,
            data: {
              requests: 0,
              errors: 0,
              by_model_channel: [],
              window: { start: 0, end: 3600, bucket_seconds: 600 },
              trend: [],
              switches: [],
              switch_reasons: [],
              affinity_by_rule: [],
            },
          })
        }
        return jsonResponse({ success: true, data: { entries: [], total: 0 } })
      })
    )

    renderPage()

    await waitFor(() =>
      expect(
        screen.getByText(
          'Run requests, or widen the time window, to collect routing statistics.'
        )
      ).toBeInTheDocument()
    )
  })

  it('explains a forbidden response instead of blanking the page', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ success: false, message: 'forbidden' }),
            { status: 403, headers: { 'Content-Type': 'application/json' } }
          )
      )
    )

    renderPage()

    await waitFor(() =>
      expect(
        screen.getByText('Administrator access required')
      ).toBeInTheDocument()
    )
  })
})
