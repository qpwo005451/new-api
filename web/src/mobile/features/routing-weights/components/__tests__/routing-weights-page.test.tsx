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
import { Toaster } from 'sonner'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { RoutingWeightsPage } from '@/mobile/features/routing-weights/components/routing-weights-page'
import { mobileQueryClient } from '@/mobile/lib/query-client'

const WEIGHTS_OPTION_KEY = 'model_weight_setting.weights'
const PRESETS_OPTION_KEY = 'model_weight_setting.presets'

const WEIGHTS = JSON.stringify([
  { channel_id: 9, model: 'gpt-5', weight: 700 },
  { channel_id: 36, model: 'gpt-5', weight: 300 },
  { channel_id: 9, model: 'glm-4.6', weight: 500 },
])

const PRESETS = JSON.stringify([
  {
    name: 'Sub first',
    model: 'gpt-5',
    weights: [
      { channel_id: 9, model: 'gpt-5', weight: 900 },
      { channel_id: 36, model: 'gpt-5', weight: 100 },
    ],
  },
])

const CHANNELS = [
  {
    id: 9,
    name: 'primary-openai',
    type: 1,
    status: 1,
    group: 'default',
    balance: 0,
    used_quota: 0,
    priority: 501,
    weight: 1000,
  },
  {
    id: 36,
    name: 'ollama',
    type: 1,
    status: 1,
    group: 'default',
    balance: 0,
    used_quota: 0,
    priority: 501,
    weight: 1000,
  },
]

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

interface FetchOverrides {
  weights?: string
  presets?: string
  channels?: unknown[]
  policyStatus?: number
}

function installFetch(overrides: FetchOverrides = {}) {
  const weights = overrides.weights ?? WEIGHTS
  const presets = overrides.presets ?? PRESETS
  const channels = overrides.channels ?? CHANNELS
  const policyStatus = overrides.policyStatus ?? 200

  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    const target = String(url)
    if (target.startsWith('/api/option/request_policy')) {
      if (init?.method === 'PATCH') {
        const body = JSON.parse(String(init.body)) as {
          options: Record<string, string>
        }
        return jsonResponse({ success: true, data: { options: body.options } })
      }
      if (policyStatus !== 200) {
        return jsonResponse(
          { success: false, message: 'forbidden' },
          policyStatus
        )
      }
      return jsonResponse({
        success: true,
        data: {
          options: {
            [WEIGHTS_OPTION_KEY]: weights,
            [PRESETS_OPTION_KEY]: presets,
          },
        },
      })
    }
    if (target.startsWith('/api/channel')) {
      return jsonResponse({
        success: true,
        data: { items: channels, total: channels.length },
      })
    }
    return jsonResponse({ success: false, message: 'not found' }, 404)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function patchCalls(fetchMock: ReturnType<typeof vi.fn>) {
  return fetchMock.mock.calls.filter(([, init]) => init?.method === 'PATCH')
}

function renderPage() {
  return render(
    <QueryClientProvider client={mobileQueryClient}>
      <RoutingWeightsPage />
      <Toaster />
    </QueryClientProvider>
  )
}

describe('RoutingWeightsPage', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.localStorage.setItem('newapi_mobile_pat', 'a'.repeat(29))
    mobileQueryClient.clear()
    vi.unstubAllGlobals()
  })

  it('groups rows by model and prints each tier share', async () => {
    installFetch()
    renderPage()

    await waitFor(() => expect(screen.getByText('gpt-5')).toBeInTheDocument())
    expect(screen.getByText('glm-4.6')).toBeInTheDocument()
    expect(screen.getByText('70%')).toBeInTheDocument()
    expect(screen.getByText('30%')).toBeInTheDocument()
    expect(screen.getByText('100%')).toBeInTheDocument()
    expect(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    ).toBeInTheDocument()
    expect(
      screen.getByRole('combobox', { name: 'Routing ratio glm-4.6' })
    ).toBeInTheDocument()
  })

  it('switches a scoped preset with one PATCH that only changes that model', async () => {
    const fetchMock = installFetch()
    renderPage()
    await screen.findByText('gpt-5')
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    )
    await user.click(await screen.findByRole('option', { name: 'Sub first' }))
    await user.click(await screen.findByRole('button', { name: 'Apply' }))

    await waitFor(() => expect(patchCalls(fetchMock)).toHaveLength(1))
    expect(JSON.parse(String(patchCalls(fetchMock)[0][1]?.body))).toEqual({
      options: {
        [WEIGHTS_OPTION_KEY]: JSON.stringify([
          { channel_id: 9, model: 'gpt-5', weight: 900 },
          { channel_id: 36, model: 'gpt-5', weight: 100 },
          { channel_id: 9, model: 'glm-4.6', weight: 500 },
        ]),
      },
    })
  })

  it('normalizes a custom ratio and cancels without writing', async () => {
    const fetchMock = installFetch()
    renderPage()
    await screen.findByText('gpt-5')
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )
    const first = await screen.findByRole('spinbutton', {
      name: 'primary-openai',
    })
    const second = screen.getByRole('spinbutton', { name: 'ollama' })
    expect(first).toHaveValue(70)
    expect(second).toHaveValue(30)

    await user.click(screen.getByRole('button', { name: 'Cancel' }))
    await waitFor(() =>
      expect(
        screen.queryByRole('spinbutton', { name: 'primary-openai' })
      ).not.toBeInTheDocument()
    )
    expect(patchCalls(fetchMock)).toHaveLength(0)

    await user.click(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )
    const firstAgain = await screen.findByRole('spinbutton', {
      name: 'primary-openai',
    })
    const secondAgain = screen.getByRole('spinbutton', { name: 'ollama' })
    await user.clear(firstAgain)
    await user.type(firstAgain, '0.4')
    await user.clear(secondAgain)
    await user.type(secondAgain, '99.6')
    await user.click(screen.getByRole('button', { name: 'Apply' }))

    await waitFor(() => expect(patchCalls(fetchMock)).toHaveLength(1))
    expect(JSON.parse(String(patchCalls(fetchMock)[0][1]?.body))).toEqual({
      options: {
        [WEIGHTS_OPTION_KEY]: JSON.stringify([
          { channel_id: 9, model: 'gpt-5', weight: 1 },
          { channel_id: 36, model: 'gpt-5', weight: 100 },
          { channel_id: 9, model: 'glm-4.6', weight: 500 },
        ]),
      },
    })
  })

  it('rejects an all-zero custom ratio without writing', async () => {
    const fetchMock = installFetch()
    renderPage()
    await screen.findByText('gpt-5')
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )
    const first = await screen.findByRole('spinbutton', {
      name: 'primary-openai',
    })
    const second = screen.getByRole('spinbutton', { name: 'ollama' })
    await user.clear(first)
    await user.clear(second)
    await user.click(screen.getByRole('button', { name: 'Apply' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Enter a ratio greater than 0.'
    )
    expect(patchCalls(fetchMock)).toHaveLength(0)
  })

  it('keeps a locked tier out of the written payload', async () => {
    const fetchMock = installFetch({
      weights: JSON.stringify([
        { channel_id: 7, model: 'gpt-5', weight: 700, priority: 800 },
        { channel_id: 9, model: 'gpt-5', priority: 501 },
        { channel_id: 36, model: 'gpt-5', priority: 501 },
      ]),
      presets: '[]',
      channels: [
        { ...CHANNELS[0], id: 7, name: 'edge', priority: 800 },
        { ...CHANNELS[0], id: 9, name: 'primary-openai', priority: 501 },
      ],
    })
    renderPage()
    await screen.findByText('gpt-5')
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )
    await user.click(screen.getByRole('button', { name: 'Apply' }))
    await waitFor(() => expect(patchCalls(fetchMock)).toHaveLength(1))

    const body = JSON.parse(String(patchCalls(fetchMock)[0][1]?.body)) as {
      options: Record<string, string>
    }
    const written = JSON.parse(body.options[WEIGHTS_OPTION_KEY]) as Array<{
      channel_id: number
      weight?: number
    }>

    expect(written).toEqual([
      { channel_id: 7, model: 'gpt-5', weight: 100, priority: 800 },
      { channel_id: 9, model: 'gpt-5', priority: 501 },
      { channel_id: 36, model: 'gpt-5', priority: 501 },
    ])
    // Neither channel of the locked tier gets an explicit weight: the known
    // raw weight is never reinterpreted as a percentage and the unknown weight
    // never becomes an explicit zero.
    expect(
      written.find((entry) => entry.channel_id === 9)?.weight
    ).toBeUndefined()
    expect(
      written.find((entry) => entry.channel_id === 36)?.weight
    ).toBeUndefined()
  })

  it('renders locked tiers read-only with the explanatory copy', async () => {
    installFetch({
      weights: JSON.stringify([
        { channel_id: 7, model: 'gpt-5', weight: 700, priority: 800 },
        { channel_id: 9, model: 'gpt-5', priority: 501 },
        { channel_id: 36, model: 'gpt-5', priority: 501 },
      ]),
      presets: '[]',
      channels: [
        { ...CHANNELS[0], id: 7, name: 'edge', priority: 800 },
        { ...CHANNELS[0], id: 9, name: 'primary-openai', priority: 501 },
      ],
    })
    renderPage()
    await screen.findByText('gpt-5')
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )

    const lockedKnown = await screen.findByRole('spinbutton', {
      name: 'primary-openai',
    })
    expect(lockedKnown).toBeDisabled()
    // The raw channel weight (1000) must never be seeded as a percentage.
    expect(lockedKnown).toHaveValue(null)
    expect(screen.getByRole('spinbutton', { name: '#36' })).toBeDisabled()
    expect(
      screen.getByText(
        'Channel data is unavailable, so this tier keeps its current weights.'
      )
    ).toBeInTheDocument()
  })

  it('validates any-positive against editable tiers only', async () => {
    const fetchMock = installFetch({
      weights: JSON.stringify([
        { channel_id: 7, model: 'gpt-5', weight: 700, priority: 800 },
        { channel_id: 9, model: 'gpt-5', weight: 700, priority: 501 },
        { channel_id: 36, model: 'gpt-5', priority: 501 },
      ]),
      presets: '[]',
      channels: [
        { ...CHANNELS[0], id: 7, name: 'edge', priority: 800 },
        { ...CHANNELS[0], id: 9, name: 'primary-openai', priority: 501 },
      ],
    })
    renderPage()
    await screen.findByText('gpt-5')
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )
    // Clearing the only editable input must fail even though the locked tier
    // still carries a raw weight of 700.
    await user.clear(screen.getByRole('spinbutton', { name: 'edge' }))
    await user.click(screen.getByRole('button', { name: 'Apply' }))

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Enter a ratio greater than 0.'
    )
    expect(patchCalls(fetchMock)).toHaveLength(0)
  })

  it('disables Apply when every tier is locked', async () => {
    installFetch({
      weights: JSON.stringify([
        { channel_id: 9, model: 'gpt-5', priority: 501 },
        { channel_id: 36, model: 'gpt-5', priority: 501 },
      ]),
      presets: '[]',
      channels: [CHANNELS[0]],
    })
    renderPage()
    await screen.findByText('gpt-5')
    const user = userEvent.setup()

    await user.click(
      screen.getByRole('combobox', { name: 'Routing ratio gpt-5' })
    )
    await user.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )

    expect(
      await screen.findByText(
        'Channel data is unavailable, so this tier keeps its current weights.'
      )
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('renders the administrator copy and no retry on a forbidden read', async () => {
    installFetch({ policyStatus: 403 })
    renderPage()

    await waitFor(() =>
      expect(
        screen.getByText('Administrator access required')
      ).toBeInTheDocument()
    )
    expect(
      screen.getByText('This page needs an administrator access token.')
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Retry' })
    ).not.toBeInTheDocument()
  })

  it('degrades an unknown channel to its id and hides the share', async () => {
    installFetch({
      weights: JSON.stringify([
        { channel_id: 9, model: 'gpt-5', priority: 501 },
        { channel_id: 36, model: 'gpt-5', priority: 501 },
      ]),
      presets: '[]',
      channels: [CHANNELS[0]],
    })
    renderPage()

    await waitFor(() => expect(screen.getByText('#36')).toBeInTheDocument())
    expect(
      screen.getByText('The share is computed inside one priority tier.')
    ).toBeInTheDocument()
    // The tier cannot be trusted while #36 has no channel weight, so both rows
    // print "-" instead of a guessed share.
    expect(screen.getAllByText('-').length).toBeGreaterThanOrEqual(2)
  })

  it('renders the override rows while the channels query is still pending', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (String(url).startsWith('/api/option/request_policy')) {
          return jsonResponse({
            success: true,
            data: {
              options: {
                [WEIGHTS_OPTION_KEY]: WEIGHTS,
                [PRESETS_OPTION_KEY]: PRESETS,
              },
            },
          })
        }
        // Never resolve the channels request: the page must not wait for it.
        return new Promise<Response>(() => {})
      })
    )
    renderPage()

    await screen.findByText('gpt-5')
    // Channel names degrade to #id, but the rows are already usable. Both
    // models use channel #9, so the fallback label appears twice.
    expect(screen.getAllByText('#9')).toHaveLength(2)
    expect(screen.getByText('#36')).toBeInTheDocument()
    expect(screen.getByText('70%')).toBeInTheDocument()
    expect(screen.getByText('30%')).toBeInTheDocument()
  })
})
