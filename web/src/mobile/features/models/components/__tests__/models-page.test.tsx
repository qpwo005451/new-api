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

import { ModelsPage } from '@/mobile/features/models/components/models-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

describe('ModelsPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  it('shows the site health counters and the failing model', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () =>
        jsonResponse({
          success: true,
          data: {
            enabled: true,
            sites: [
              {
                site: { id: 1, name: 'primary', enabled: true },
                summary: {
                  score: 80,
                  health: 'degraded',
                  models: [
                    {
                      model_name: 'claude-5',
                      status: 'unavailable',
                      latest_status: 'failure',
                      latest_failure_type: 'timeout',
                      latest_error_summary: 'upstream timeout',
                      weight: 50,
                      stale: false,
                    },
                  ],
                },
                channel_ids: [3],
                latest_observed_at: 1000,
                freshness_seconds: 120,
              },
            ],
          },
        })
      )
    )

    render(
      <QueryClientProvider client={mobileQueryClient}>
        <ModelsPage />
      </QueryClientProvider>
    )

    // The failing model is rendered both as a site row and in the failure
    // list, so the query must tolerate more than one match.
    await waitFor(() =>
      expect(screen.getAllByText('claude-5').length).toBeGreaterThan(0)
    )
    expect(screen.getByText('Degraded')).toBeInTheDocument()
    expect(screen.getByText('upstream timeout')).toBeInTheDocument()
  })

  it('explains that the monitor is disabled', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ success: true, data: { enabled: false, sites: [] } })))

    render(
      <QueryClientProvider client={mobileQueryClient}>
        <ModelsPage />
      </QueryClientProvider>
    )

    await waitFor(() => expect(screen.getByText('Model monitoring is disabled on this instance.')).toBeInTheDocument())
  })
})
