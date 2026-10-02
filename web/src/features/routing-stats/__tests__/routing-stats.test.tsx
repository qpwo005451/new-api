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
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { RoutingStatsPage } from '../routing-stats'
import type { ChannelAffinityBindings, RoutingStats } from '../types'

vi.mock('@visactor/react-vchart', () => ({ VChart: () => null }))
vi.mock('@visactor/vchart', () => ({
  ThemeManager: { setCurrentTheme: () => undefined },
}))

function statsFixture(overrides: Partial<RoutingStats> = {}): RoutingStats {
  return {
    requests: 10,
    errors: 1,
    by_model_channel: [
      {
        model_name: 'deepseek-v4.1-flash',
        channel_id: 9,
        channel_name: 'input-0.1X',
        requests: 6,
        errors: 1,
        avg_use_time: 1.5,
        priority: 500,
        weight: 80,
      },
      {
        model_name: 'deepseek-v4.1-flash',
        channel_id: 47,
        channel_name: 'commandcode',
        requests: 4,
        errors: 0,
        avg_use_time: 2.5,
        priority: 500,
        weight: 20,
      },
    ],
    window: { start: 1000, end: 4600, bucket_seconds: 120 },
    trend: [{ timestamp: 1000, requests: 10, switched: 2 }],
    scanned: 10,
    truncated: false,
    switched: 2,
    switched_success: 1,
    switched_failed: 1,
    sticky: 3,
    switches: [{ from: 9, to: 47, count: 2 }],
    switch_reasons: [{ reason: 'channel_error', count: 2 }],
    affinity_by_rule: [
      {
        rule_name: 'deepseek glm session stickiness',
        sticky_requests: 3,
        distinct_keys: 2,
      },
    ],
    ...overrides,
  }
}

function bindingsFixture(): ChannelAffinityBindings {
  return {
    enabled: true,
    total: 2,
    unknown: 0,
    truncated: false,
    entries: [
      {
        rule_name: 'deepseek glm session stickiness',
        model_name: 'deepseek-v4.1-flash',
        using_group: 'default',
        key_hint: 'sess...a',
        key_fingerprint: 'aaaaaaaa',
        channel_id: 9,
        channel_name: 'input-0.1X',
      },
      {
        rule_name: 'deepseek glm session stickiness',
        model_name: 'deepseek-v4.1-flash',
        using_group: 'default',
        key_hint: 'sess...b',
        key_fingerprint: 'bbbbbbbb',
        channel_id: 47,
        channel_name: 'commandcode',
      },
    ],
  }
}

interface ApiRoute {
  success: boolean
  message?: string
  data?: unknown
}

let client: QueryClient
let statsResponse: ApiRoute
let statsShouldFail: boolean
let statsRequests: { params?: Record<string, unknown> }[]
let bindingsResponse: ChannelAffinityBindings

beforeEach(() => {
  client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  statsResponse = { success: true, data: statsFixture() }
  statsShouldFail = false
  statsRequests = []
  bindingsResponse = bindingsFixture()

  vi.spyOn(api, 'get').mockImplementation((async (
    url: string,
    config?: { params?: Record<string, unknown> }
  ) => {
    if (url === '/api/log/routing_stats') {
      statsRequests.push({ params: config?.params })
      if (statsShouldFail) throw new Error('request failed')
      return { data: statsResponse }
    }
    if (url === '/api/log/channel_affinity_bindings') {
      return { data: { success: true, data: bindingsResponse } }
    }
    throw new Error(`unexpected request: ${url}`)
  }) as unknown as typeof api.get)
})

afterEach(() => {
  client.clear()
  vi.restoreAllMocks()
})

function renderPage() {
  return render(
    <QueryClientProvider client={client}>
      <RoutingStatsPage />
    </QueryClientProvider>
  )
}

describe('RoutingStatsPage', () => {
  it('shows the window summary beside the observed and configured split', async () => {
    renderPage()

    expect(await screen.findByText('Window summary')).toBeInTheDocument()
    // Observed share of channel 9 differs from its configured 80% weight, which
    // is exactly the comparison the page exists for.
    expect(screen.getByText('60%')).toBeInTheDocument()
    expect(screen.getByText('80%')).toBeInTheDocument()
    expect(screen.getAllByText('input-0.1X').length).toBeGreaterThan(0)
    expect(
      screen.getByRole('combobox', { name: 'Time window' })
    ).toHaveTextContent('Last hour')
  })

  it('warns when the decision trail inspection did not cover the whole window', async () => {
    statsResponse = {
      success: true,
      data: statsFixture({ truncated: true, scanned: 50000 }),
    }
    renderPage()

    expect(
      await screen.findByText('Partial switch analysis')
    ).toBeInTheDocument()
    expect(screen.queryByText('Window summary')).toBeInTheDocument()
  })

  it('reports an empty window instead of rendering empty cards', async () => {
    statsResponse = {
      success: true,
      data: statsFixture({ requests: 0, by_model_channel: [], scanned: 0 }),
    }
    renderPage()

    expect(
      await screen.findByText('No routing data in the selected window.')
    ).toBeInTheDocument()
    expect(screen.queryByText('Window summary')).not.toBeInTheDocument()
  })

  it('shows a retryable error when the statistics request fails', async () => {
    statsShouldFail = true
    renderPage()

    expect(
      await screen.findByText('Failed to load routing statistics')
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument()
  })

  it('shows which channel each model holds sessions on right now', async () => {
    renderPage()

    expect(
      (await screen.findAllByText('Current session allocation')).length
    ).toBeGreaterThan(0)
    expect(screen.getByText('sess...a')).toBeInTheDocument()
    expect(screen.getAllByText('commandcode').length).toBeGreaterThan(0)
  })

  it('says so when the live session list is capped', async () => {
    bindingsResponse = { ...bindingsFixture(), truncated: true }
    renderPage()

    expect(
      await screen.findByText(
        'More sessions are pinned to a channel than the list below shows.'
      )
    ).toBeInTheDocument()
  })

  it('reloads the window for the model typed into the filter', async () => {
    const user = userEvent.setup()
    renderPage()
    await screen.findByText('Window summary')

    await user.type(screen.getByLabelText('Filter by model'), 'glm-5.3-flash')
    await user.click(screen.getByRole('button', { name: 'Filter' }))

    await waitFor(() => {
      const last = statsRequests.at(-1)
      expect(last?.params?.model_name).toBe('glm-5.3-flash')
    })
  })
})
