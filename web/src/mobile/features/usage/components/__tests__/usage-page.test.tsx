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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { UsagePage } from '@/mobile/features/usage/components/usage-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'Content-Type': 'application/json' } })
}

function renderPage() {
  render(
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

  it('shows totals from the quota data aggregate', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url.startsWith('/api/log/self/stat')) {
          return jsonResponse({ success: true, data: { quota: 1650, rpm: 3, tpm: 400 } })
        }
        if (url.startsWith('/api/data/self')) {
          return jsonResponse({
            success: true,
            data: [{ model_name: 'gpt-5', created_at: 1, count: 16, quota: 1650, token_used: 1530 }],
          })
        }
        return jsonResponse({ success: true, data: { items: [], total: 0 } })
      })
    )

    renderPage()

    await waitFor(() => expect(screen.getByText('16')).toBeInTheDocument())
    // formatTokens shortens thousands, and both the KPI card and the model row
    // render 1530 tokens, so more than one matching node is expected.
    expect(screen.getAllByText('1.5K').length).toBeGreaterThan(0)
    expect(screen.getByText('gpt-5')).toBeInTheDocument()
    expect(screen.getByText('Summary data updates every few minutes.')).toBeInTheDocument()
  })

  it('switches the scope to the whole instance', async () => {
    const fetchMock = vi.fn(async (_url: string) =>
      jsonResponse({ success: true, data: { quota: 0, rpm: 0, tpm: 0 } })
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    renderPage()
    await user.click(screen.getByRole('button', { name: 'All' }))

    await waitFor(() => expect(fetchMock.mock.calls.some(([url]) => String(url).startsWith('/api/log/stat'))).toBe(true))
  })

  it('shows the empty state when the range has no data', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ success: true, data: [] })))

    renderPage()

    await waitFor(() => expect(screen.getByText('Nothing to show for this range.')).toBeInTheDocument())
  })
})
