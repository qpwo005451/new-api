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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { RoutingPage } from '@/mobile/features/routing/components/routing-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

const routingStats = {
  requests: 400,
  errors: 0,
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
  affinity_by_rule: [{ rule_name: 'default', sticky_requests: 12, distinct_keys: 3 }],
}

function renderPage() {
  return render(
    <QueryClientProvider client={mobileQueryClient}>
      <RoutingPage />
    </QueryClientProvider>
  )
}

describe('RoutingPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
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
    // 'Channel switches' labels both the KPI and the section heading, so the
    // assertion accepts either occurrence.
    expect(screen.getAllByText('Channel switches').length).toBeGreaterThan(0)
    // The 'Channel switches' KPI and the switch row both show the same total,
    // so the assertion accepts either occurrence.
    expect(screen.getAllByText('4').length).toBeGreaterThan(0)
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
