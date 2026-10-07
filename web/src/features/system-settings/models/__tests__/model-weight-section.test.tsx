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
import {
  createMemoryHistory,
  createRootRoute,
  createRouter,
  RouterContextProvider,
} from '@tanstack/react-router'
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { useState } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { ModelWeightSection } from '../model-weight-section'

const MODEL = 'deepseek-v4.1-flash'
const OTHER_MODEL = 'glm-4.6'

type MockChannel = {
  id: number
  name: string
  status: number
  priority: number
  weight: number
  models: string
  model_mapping: string
}

const channelState = vi.hoisted(() => ({
  error: null as Error | null,
  items: [] as MockChannel[],
}))

vi.mock('@/features/channels/api', () => ({
  getChannels: vi.fn(async () => {
    if (channelState.error) throw channelState.error
    return {
      success: true,
      data: {
        items: channelState.items,
        total: channelState.items.length,
        page: 1,
        page_size: 100,
      },
    }
  }),
}))

function mockChannel(overrides: Partial<MockChannel> & { id: number }) {
  return {
    name: `channel-${overrides.id}`,
    status: 1,
    priority: 500,
    weight: 100,
    models: MODEL,
    model_mapping: '',
    ...overrides,
  }
}

let client: QueryClient
let weights: string
let presetsRaw: string

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  channelState.error = null
  channelState.items = [mockChannel({ id: 9 }), mockChannel({ id: 36 })]
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  weights = JSON.stringify([{ channel_id: 9, model: MODEL, weight: 20 }])
  presetsRaw = '[]'
  vi.spyOn(api, 'patch').mockResolvedValue({
    data: {
      success: true,
      data: { options: { 'model_weight_setting.weights': weights } },
    },
  })
})

afterEach(async () => {
  cleanup()
  client.clear()
  vi.restoreAllMocks()
  await i18next.changeLanguage('en')
})

function Workspace() {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  return (
    <>
      <div ref={setContainer} />
      <SettingsPageProvider actionsContainer={container}>
        <ModelWeightSection
          defaultValues={{
            'model_weight_setting.weights': weights,
            'model_weight_setting.presets': presetsRaw,
          }}
        />
      </SettingsPageProvider>
    </>
  )
}

function show() {
  const router = createRouter({
    routeTree: createRootRoute(),
    history: createMemoryHistory({ initialEntries: ['/'] }),
  })
  render(
    <QueryClientProvider client={client}>
      <RouterContextProvider router={router}>
        <Workspace />
      </RouterContextProvider>
    </QueryClientProvider>
  )
}

function shareInput(model: string, channelId: number) {
  return screen.getByRole('spinbutton', {
    name: `Share ${model} #${channelId}`,
  })
}

function savedWeightsPayload() {
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  return JSON.parse(
    payload.options['model_weight_setting.weights']
  ) as Array<Record<string, unknown>>
}

async function save() {
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))
  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
}

describe('per-model allocation cards', () => {
  it('lists every enabled channel that serves a model, not only the override rows', async () => {
    weights = JSON.stringify([{ channel_id: 9, model: MODEL, weight: 700 }])
    channelState.items = [
      mockChannel({ id: 9, name: 'channel-nine', weight: 300 }),
      mockChannel({ id: 36, name: 'channel-thirty-six', weight: 700 }),
    ]
    show()

    expect(await screen.findByText('channel-nine')).toBeVisible()
    expect(screen.getByText('channel-thirty-six')).toBeVisible()
  })

  it('hides a disabled channel and preserves its override unchanged on save', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 100 },
      { channel_id: 47, model: MODEL, weight: 5 },
    ])
    channelState.items = [
      mockChannel({ id: 9, name: 'channel-nine', weight: 300 }),
      mockChannel({ id: 36, name: 'channel-thirty-six', weight: 700 }),
      mockChannel({
        id: 47,
        name: 'channel-forty-seven',
        status: 0,
        weight: 100,
      }),
    ]
    show()

    expect(await screen.findByText('channel-nine')).toBeVisible()
    expect(screen.queryByText('channel-forty-seven')).not.toBeInTheDocument()

    fireEvent.change(shareInput(MODEL, 9), { target: { value: '70' } })
    fireEvent.change(shareInput(MODEL, 36), { target: { value: '30' } })
    await save()

    const entries = savedWeightsPayload()
    expect(entries).toHaveLength(3)
    expect(entries).toContainEqual({
      channel_id: 9,
      model: MODEL,
      weight: 70,
    })
    expect(entries).toContainEqual({
      channel_id: 36,
      model: MODEL,
      weight: 30,
    })
    expect(entries).toContainEqual({
      channel_id: 47,
      model: MODEL,
      weight: 5,
    })
  })

  it('converts each tier percentages to weights with the documented rounding', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 700, priority: 500 },
      { channel_id: 36, model: MODEL, weight: 300, priority: 500 },
      { channel_id: 47, model: MODEL, weight: 100, priority: 400 },
      { channel_id: 48, model: MODEL, weight: 200, priority: 400 },
    ])
    channelState.items = [9, 36, 47, 48].map((id) =>
      mockChannel({ id, name: `channel-${id}` })
    )
    show()

    await screen.findByText('channel-9')
    fireEvent.change(shareInput(MODEL, 9), { target: { value: '0.4' } })
    fireEvent.change(shareInput(MODEL, 36), { target: { value: '99.6' } })
    fireEvent.change(shareInput(MODEL, 47), { target: { value: '33' } })
    fireEvent.change(shareInput(MODEL, 48), { target: { value: '67' } })
    await save()

    const entries = savedWeightsPayload()
    expect(entries).toEqual([
      { channel_id: 9, model: MODEL, weight: 1, priority: 500 },
      { channel_id: 36, model: MODEL, weight: 100, priority: 500 },
      { channel_id: 47, model: MODEL, weight: 33, priority: 400 },
      { channel_id: 48, model: MODEL, weight: 67, priority: 400 },
    ])
  })

  it('splits each priority tier evenly when Average split is used', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 700, priority: 500 },
      { channel_id: 36, model: MODEL, weight: 300, priority: 500 },
      { channel_id: 47, model: MODEL, weight: 100, priority: 400 },
      { channel_id: 48, model: MODEL, weight: 900, priority: 400 },
    ])
    channelState.items = [9, 36, 47, 48].map((id) =>
      mockChannel({ id, name: `channel-${id}` })
    )
    show()

    await screen.findByText('channel-9')
    await userEvent.click(screen.getByRole('button', { name: 'Average split' }))

    expect(shareInput(MODEL, 9)).toHaveValue(50)
    expect(shareInput(MODEL, 36)).toHaveValue(50)
    expect(shareInput(MODEL, 47)).toHaveValue(50)
    expect(shareInput(MODEL, 48)).toHaveValue(50)
  })

  it('fills the card from a preset without writing until Save', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 700 },
      { channel_id: 36, model: MODEL, weight: 300 },
    ])
    presetsRaw = JSON.stringify([
      {
        name: 'even',
        model: MODEL,
        weights: [
          { channel_id: 9, model: MODEL, weight: 200 },
          { channel_id: 36, model: MODEL, weight: 800 },
        ],
      },
    ])
    channelState.items = [mockChannel({ id: 9 }), mockChannel({ id: 36 })]
    show()

    await screen.findByText('channel-9')
    await userEvent.click(
      screen.getByRole('combobox', { name: `Preset ${MODEL}` })
    )
    await userEvent.click(await screen.findByRole('option', { name: 'even' }))

    expect(api.patch).not.toHaveBeenCalled()
    expect(shareInput(MODEL, 9)).toHaveValue(20)
    expect(shareInput(MODEL, 36)).toHaveValue(80)

    await save()
    expect(savedWeightsPayload()).toEqual([
      { channel_id: 9, model: MODEL, weight: 20 },
      { channel_id: 36, model: MODEL, weight: 80 },
    ])
  })

  it('never rewrites an override that belongs to another model', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 700 },
      { channel_id: 36, model: MODEL, weight: 300 },
      { channel_id: 9, model: OTHER_MODEL, weight: 123 },
    ])
    channelState.items = [
      mockChannel({ id: 9, models: `${MODEL},${OTHER_MODEL}` }),
      mockChannel({ id: 36, models: MODEL }),
    ]
    show()

    await screen.findByRole('spinbutton', {
      name: `Share ${MODEL} #9`,
    })
    fireEvent.change(shareInput(MODEL, 9), { target: { value: '60' } })
    fireEvent.change(shareInput(MODEL, 36), { target: { value: '40' } })
    await save()

    const entries = savedWeightsPayload()
    expect(entries).toContainEqual({
      channel_id: 9,
      model: OTHER_MODEL,
      weight: 123,
    })
    expect(entries).toContainEqual({ channel_id: 9, model: MODEL, weight: 60 })
    expect(entries).toContainEqual({
      channel_id: 36,
      model: MODEL,
      weight: 40,
    })
  })
})

describe('card presentation', () => {
  it('labels a single tier with its role and total', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 700 },
      { channel_id: 36, model: MODEL, weight: 300 },
    ])
    channelState.items = [mockChannel({ id: 9 }), mockChannel({ id: 36 })]
    show()

    expect(await screen.findByText('channel-9')).toBeVisible()
    expect(screen.getByText('Priority 500')).toBeVisible()
    expect(screen.getByText('Participating')).toBeVisible()
    expect(screen.getByText('Total')).toBeVisible()
  })

  it('keeps the percent suffix inside the share field', async () => {
    weights = JSON.stringify([{ channel_id: 9, model: MODEL, weight: 100 }])
    channelState.items = [mockChannel({ id: 9 })]
    show()

    const share = await screen.findByRole('spinbutton', {
      name: `Share ${MODEL} #9`,
    })
    expect(share.parentElement).toHaveTextContent('%')
  })

  it('shows one tier subheader per priority when the model has several', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 700, priority: 500 },
      { channel_id: 36, model: MODEL, weight: 300, priority: 400 },
    ])
    channelState.items = [
      mockChannel({ id: 9, priority: 500 }),
      mockChannel({ id: 36, priority: 400 }),
    ]
    show()

    expect(await screen.findByText('Priority 500')).toBeVisible()
    expect(screen.getByText('Priority 400')).toBeVisible()
  })

  it('rebalances the rest of the tier so it still totals 100 after an edit', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 700 },
      { channel_id: 36, model: MODEL, weight: 300 },
    ])
    channelState.items = [mockChannel({ id: 9 }), mockChannel({ id: 36 })]
    show()

    await screen.findByText('channel-9')
    expect(screen.getByText('Total')).toBeVisible()
    expect(screen.getByText('100%')).toBeVisible()

    fireEvent.change(shareInput(MODEL, 36), { target: { value: '70' } })
    expect(shareInput(MODEL, 36)).toHaveValue(70)
    expect(shareInput(MODEL, 9)).toHaveValue(30)
    // The tier still totals 100, so the total stays quiet (not destructive).
    expect(screen.getByText('100%')).toBeVisible()
    expect(screen.getByText('100%')).not.toHaveClass('text-destructive')
  })

  it('reveals editable priority and raw weight fields behind Advanced', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 700 },
      { channel_id: 36, model: MODEL, weight: 300 },
    ])
    channelState.items = [mockChannel({ id: 9 }), mockChannel({ id: 36 })]
    show()

    await screen.findByText('channel-9')
    expect(
      screen.queryByRole('spinbutton', {
        name: `Priority ${MODEL} #9`,
      })
    ).not.toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Advanced' }))
    expect(
      screen.getByRole('spinbutton', { name: `Priority ${MODEL} #9` })
    ).toHaveValue(null)
    expect(
      screen.getByRole('spinbutton', { name: `Weight ${MODEL} #9` })
    ).toHaveValue(70)
  })

  it('shows an empty state when no enabled channel serves the model', async () => {
    weights = JSON.stringify([{ channel_id: 9, model: MODEL, weight: 20 }])
    channelState.items = [mockChannel({ id: 9, status: 0 })]
    show()

    expect(
      await screen.findByText('No enabled channel serves this model.')
    ).toBeVisible()
  })

  it('keeps the saved preset selected while the card is dirty', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, priority: 500, weight: 700 },
    ])
    presetsRaw = JSON.stringify([
      {
        name: 'ollama',
        model: MODEL,
        weights: [
          {
            channel_id: 9,
            model: MODEL,
            priority: 500,
            weight: 700,
          },
        ],
      },
    ])
    channelState.items = [mockChannel({ id: 9 })]
    show()

    const select = await screen.findByRole('combobox', {
      name: `Preset ${MODEL}`,
    })
    expect(select).toHaveTextContent('ollama')

    await screen.findByRole('spinbutton', { name: `Share ${MODEL} #9` })
    fireEvent.change(shareInput(MODEL, 9), { target: { value: '5' } })

    expect(await screen.findByText('Unsaved changes')).toBeVisible()
    expect(select).toHaveTextContent('ollama')
  })
})

describe('preset management', () => {
  it('marks the matching global preset as active', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, priority: 500, weight: 50 },
    ])
    presetsRaw = JSON.stringify([
      {
        name: '默认',
        weights: [
          {
            channel_id: 9,
            model: MODEL,
            priority: 500,
            weight: 50,
          },
        ],
      },
    ])
    channelState.items = [mockChannel({ id: 9 })]
    show()

    const preset = await screen.findByRole('button', { name: '默认' })
    await waitFor(() => expect(preset).toHaveAttribute('aria-pressed', 'true'))
  })

  it('lists only global presets in the quick-switch bar', async () => {
    weights = JSON.stringify([{ channel_id: 9, model: MODEL, weight: 20 }])
    presetsRaw = JSON.stringify([
      { name: 'global-one', weights: [] },
      { name: 'scoped-one', model: MODEL, weights: [] },
    ])
    channelState.items = [mockChannel({ id: 9 })]
    show()

    expect(
      await screen.findByRole('button', { name: 'global-one' })
    ).toBeVisible()
    expect(
      screen.queryByRole('button', { name: 'scoped-one' })
    ).not.toBeInTheDocument()
  })

  it('saves the current draft as a named global preset', async () => {
    weights = JSON.stringify([{ channel_id: 9, model: MODEL, weight: 20 }])
    channelState.items = [mockChannel({ id: 9, weight: 20 })]
    show()

    await screen.findByText('channel-9')
    fireEvent.change(shareInput(MODEL, 9), { target: { value: '30' } })
    await userEvent.click(screen.getByRole('button', { name: 'Manage presets' }))
    await userEvent.type(
      await screen.findByRole('textbox', { name: 'Preset name' }),
      'edited'
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Save current as preset' })
    )

    await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
    const payload = vi.mocked(api.patch).mock.calls[0][1] as {
      options: Record<string, string>
    }
    expect(JSON.parse(payload.options['model_weight_setting.presets'])).toEqual(
      [
        {
          name: 'edited',
          weights: [{ channel_id: 9, model: MODEL, weight: 30 }],
        },
      ]
    )
  })

  it('saves a scoped preset with only the chosen model rows', async () => {
    weights = JSON.stringify([
      { channel_id: 9, model: MODEL, weight: 20 },
      { channel_id: 36, model: OTHER_MODEL, weight: 5 },
    ])
    channelState.items = [
      mockChannel({ id: 9, models: MODEL }),
      mockChannel({ id: 36, models: OTHER_MODEL }),
    ]
    show()

    await screen.findByText('channel-9')
    await userEvent.click(screen.getByRole('button', { name: 'Manage presets' }))
    await userEvent.type(
      await screen.findByRole('textbox', { name: 'Preset name' }),
      'scoped'
    )
    await userEvent.click(
      screen.getByRole('combobox', { name: 'Preset scope' })
    )
    await userEvent.click(
      await screen.findByRole('option', { name: `Only ${MODEL}` })
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Save current as preset' })
    )

    await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
    const payload = vi.mocked(api.patch).mock.calls[0][1] as {
      options: Record<string, string>
    }
    expect(JSON.parse(payload.options['model_weight_setting.presets'])).toEqual(
      [
        {
          name: 'scoped',
          model: MODEL,
          weights: [{ channel_id: 9, model: MODEL, weight: 20 }],
        },
      ]
    )
  })

  it('deletes a preset by scope and name', async () => {
    weights = '[]'
    presetsRaw = JSON.stringify([
      { name: 'dup', weights: [] },
      { name: 'dup', model: MODEL, weights: [] },
    ])
    channelState.items = []
    show()

    await userEvent.click(screen.getByRole('button', { name: 'Manage presets' }))
    const removeButtons = await screen.findAllByRole('button', {
      name: 'Remove',
    })
    await userEvent.click(removeButtons[1])
    await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))

    await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
    const payload = vi.mocked(api.patch).mock.calls[0][1] as {
      options: Record<string, string>
    }
    expect(JSON.parse(payload.options['model_weight_setting.presets'])).toEqual(
      [{ name: 'dup', weights: [] }]
    )
  })
})
