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
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Toaster } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ChannelsPage } from '@/mobile/features/channels/components/channels-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function listResponse(items: unknown[]): Response {
  return jsonResponse({
    success: true,
    data: { items, total: items.length, page: 1, page_size: 50 },
  })
}

const CHANNEL = {
  id: 7,
  name: 'primary-openai',
  type: 1,
  status: 1,
  group: 'default',
  balance: 12.5,
  used_quota: 0,
  priority: 0,
  weight: 10,
}

function renderPage() {
  render(
    <QueryClientProvider client={mobileQueryClient}>
      <ChannelsPage />
      {/* The production shell mounts this in MobileProviders; without it the
          toast assertions below would pass vacuously. */}
      <Toaster />
    </QueryClientProvider>
  )
}

describe('ChannelsPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  it('lists channels with their status label', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => listResponse([CHANNEL]))
    )

    renderPage()

    await waitFor(() =>
      expect(screen.getByText('primary-openai')).toBeInTheDocument()
    )
    // Scope to the row so the always-rendered "Enabled" status filter chip
    // cannot make this assertion pass on its own.
    const row = screen.getByText('primary-openai').closest('li')
    expect(row).not.toBeNull()
    expect(row).toHaveTextContent('#7 · default · Enabled')
    expect(
      within(row as HTMLElement).getByRole('switch', {
        name: 'Enabled primary-openai',
      })
    ).toBeInTheDocument()
  })

  it('asks for confirmation and posts the manual-disabled status', async () => {
    const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
      if (String(url).startsWith('/api/channel/7/status')) {
        return jsonResponse({ success: true, data: true })
      }
      if (init?.method === 'GET' || init?.method === undefined) {
        return listResponse([CHANNEL])
      }
      return jsonResponse({ success: true, data: null })
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    renderPage()
    await waitFor(() =>
      expect(screen.getByText('primary-openai')).toBeInTheDocument()
    )

    await user.click(
      screen.getByRole('switch', { name: 'Enabled primary-openai' })
    )
    await user.click(
      await screen.findByRole('button', { name: 'Disable channel' })
    )

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([url]) =>
        String(url).startsWith('/api/channel/7/status')
      )
      expect(call?.[1]?.method).toBe('POST')
      expect(call?.[1]?.body).toBe('{"status":2}')
    })
  })

  it('enables an auto-disabled channel by posting status 1', async () => {
    const autoDisabled = { ...CHANNEL, status: 3 }
    const fetchMock = vi.fn(async (url: string, _init?: RequestInit) => {
      if (String(url).startsWith('/api/channel/7/status')) {
        return jsonResponse({ success: true, data: true })
      }
      return listResponse([autoDisabled])
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    renderPage()
    await waitFor(() =>
      expect(screen.getByText('primary-openai')).toBeInTheDocument()
    )

    await user.click(
      screen.getByRole('switch', { name: 'Auto disabled primary-openai' })
    )
    await user.click(
      await screen.findByRole('button', { name: 'Enable channel' })
    )

    await waitFor(() => {
      const call = fetchMock.mock.calls.find(([url]) =>
        String(url).startsWith('/api/channel/7/status')
      )
      // Status 3 is rejected by the write endpoint, so the toggle must send 1.
      expect(call?.[1]?.body).toBe('{"status":1}')
    })
  })

  it('keeps the switch on the server status and reports a rejected write', async () => {
    const fetchMock = vi.fn(async (url: string, _init?: RequestInit) => {
      if (String(url).startsWith('/api/channel/7/status')) {
        return jsonResponse({ success: false, message: 'forbidden' }, 403)
      }
      return listResponse([CHANNEL])
    })
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    renderPage()
    await waitFor(() =>
      expect(screen.getByText('primary-openai')).toBeInTheDocument()
    )

    await user.click(
      screen.getByRole('switch', { name: 'Enabled primary-openai' })
    )
    await user.click(
      await screen.findByRole('button', { name: 'Disable channel' })
    )

    // Visible feedback for the failure.
    await waitFor(() =>
      expect(
        screen.getByText('Administrator access required')
      ).toBeInTheDocument()
    )
    // No optimistic flip survived: the switch still reflects the server row.
    await waitFor(() =>
      expect(
        screen.getByRole('switch', { name: 'Enabled primary-openai' })
      ).toBeChecked()
    )
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument()
  })

  it('filters the list by channel name', async () => {
    const fetchMock = vi.fn(async (_url: string) => listResponse([CHANNEL]))
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()

    renderPage()
    await waitFor(() =>
      expect(screen.getByText('primary-openai')).toBeInTheDocument()
    )

    await user.type(screen.getByLabelText('Search channels'), 'openai')

    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([url]) =>
          String(url).includes('keyword=openai')
        )
      ).toBe(true)
    )
    // `/api/channel/` has no name filter; the search endpoint is the only one
    // that matches on the channel name.
    expect(
      fetchMock.mock.calls.some(([url]) =>
        String(url).startsWith('/api/channel/search')
      )
    ).toBe(true)
  })

  it('shows an empty state when no channel matches', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => listResponse([]))
    )

    renderPage()

    await waitFor(() =>
      expect(screen.getByText('No channels')).toBeInTheDocument()
    )
  })
})
