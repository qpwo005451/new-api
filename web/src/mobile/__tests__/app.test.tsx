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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { MobileApp } from '@/mobile/app'
import { STATUS_QUERY_KEY } from '@/mobile/lib/status'

const testQueryClient = new QueryClient({
  defaultOptions: { queries: { retry: false } },
})

function renderApp() {
  return render(
    <QueryClientProvider client={testQueryClient}>
      <MobileApp />
    </QueryClientProvider>
  )
}

describe('MobileApp', () => {
  beforeEach(() => {
    window.location.hash = ''
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    testQueryClient.clear()
    // Seeding the shared status query keeps the shell from calling /api/status
    // while the currency helpers fall back to their documented USD defaults.
    testQueryClient.setQueryData(STATUS_QUERY_KEY, {})
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders the mobile console heading', () => {
    renderApp()

    expect(
      screen.getByRole('heading', { name: 'Mobile console' })
    ).toBeInTheDocument()
  })

  it('names an unreachable gateway instead of rendering a broken shell', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => {
        throw new TypeError('Failed to fetch')
      })
    )
    // Leave the status query unseeded so the shell actually calls /api/status.
    testQueryClient.clear()
    renderApp()

    await waitFor(() =>
      expect(screen.getByText('Connection failed')).toBeInTheDocument()
    )
    expect(
      screen.queryByRole('heading', { name: 'Mobile console' })
    ).not.toBeInTheDocument()
  })

  it('renders the model routing settings page only when its tab is selected', async () => {
    window.location.hash = '#/routing-weights'
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).startsWith('/api/option/request_policy')) {
          return new Response(
            JSON.stringify({
              success: true,
              data: {
                options: {
                  'model_weight_setting.weights': JSON.stringify([
                    { channel_id: 9, model: 'gpt-5', weight: 700 },
                  ]),
                  'model_weight_setting.presets': '[]',
                },
              },
            }),
            { status: 200, headers: { 'Content-Type': 'application/json' } }
          )
        }
        return new Response(
          JSON.stringify({ success: true, data: { items: [], total: 0 } }),
          { status: 200, headers: { 'Content-Type': 'application/json' } }
        )
      })
    )

    renderApp()

    await waitFor(() => expect(screen.getByText('gpt-5')).toBeInTheDocument())
    expect(screen.getByRole('tab', { name: 'Model routing' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    // The read-only routing statistics page must not be mounted.
    expect(screen.queryByText('Traffic by model')).not.toBeInTheDocument()
  })
})
