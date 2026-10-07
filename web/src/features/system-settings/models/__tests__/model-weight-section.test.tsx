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

import type { Channel } from '@/features/channels/types'
import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import type { ModelWeightOverrideEntry } from '../model-weight-presets'
import {
  applyCustomRatioWeights,
  buildCustomRatioState,
} from '../model-weight-rows'
import { ModelWeightSection } from '../model-weight-section'

const channelState = vi.hoisted(() => ({ error: null as Error | null }))

vi.mock('@/features/channels/api', () => ({
  getChannels: vi.fn(async () => {
    if (channelState.error) throw channelState.error
    return {
      success: true,
      data: {
        items: [
          { id: 9, name: 'channel-nine', priority: 500, weight: 100 },
          { id: 36, name: 'channel-thirty-six', priority: 400, weight: 50 },
        ],
        total: 2,
        page: 1,
        page_size: 100,
      },
    }
  }),
}))

let client: QueryClient
let weights: string
let presetsRaw: string

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  channelState.error = null
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
  ])
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

it('renders existing overrides and saves an added row', async () => {
  show()

  expect(await screen.findByText('#9 - channel-nine')).toBeVisible()
  expect(screen.getByRole('textbox', { name: 'Model' })).toHaveValue(
    'deepseek-v4.1-flash'
  )
  expect(screen.getByRole('spinbutton', { name: 'Weight' })).toHaveValue(20)

  await userEvent.click(screen.getByRole('button', { name: 'Add override' }))
  const channels = screen.getAllByRole('combobox', { name: 'Channel' })
  await userEvent.click(channels[1])
  await userEvent.click(
    await screen.findByRole('option', { name: '#36 - channel-thirty-six' })
  )
  fireEvent.change(screen.getAllByRole('textbox', { name: 'Model' })[1], {
    target: { value: 'glm-5.3-flash' },
  })
  fireEvent.change(screen.getAllByRole('spinbutton', { name: 'Weight' })[1], {
    target: { value: '5' },
  })

  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.weights'])).toEqual([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
    { channel_id: 36, model: 'glm-5.3-flash', weight: 5 },
  ])
})

it('removes an override and saves the remaining rows', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
    { channel_id: 36, model: 'glm-5.3-flash', weight: 5 },
  ])
  show()

  expect(await screen.findByText('#9 - channel-nine')).toBeVisible()
  await userEvent.click(screen.getAllByRole('button', { name: 'Remove' })[0])
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.weights'])).toEqual([
    { channel_id: 36, model: 'glm-5.3-flash', weight: 5 },
  ])
})

it('renders an existing priority and saves a priority-only override', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500 },
  ])
  show()

  expect(await screen.findByText('#9 - channel-nine')).toBeVisible()
  const priority = screen.getByRole('spinbutton', { name: 'Priority' })
  expect(priority).toHaveValue(500)
  expect(screen.getByRole('spinbutton', { name: 'Weight' })).toHaveValue(null)

  fireEvent.change(priority, { target: { value: '497' } })
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.weights'])).toEqual([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 497 },
  ])
})

it('drops rows that set neither a weight nor a priority', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
  ])
  show()

  expect(await screen.findByText('#9 - channel-nine')).toBeVisible()
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Weight' }), {
    target: { value: '' },
  })
  await userEvent.click(screen.getByRole('button', { name: 'Save Changes' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.weights'])).toEqual(
    []
  )
})

it('applies a preset with one click after confirmation', async () => {
  weights = '[]'
  presetsRaw = JSON.stringify([
    {
      name: '全部 ollama',
      weights: [
        {
          channel_id: 9,
          model: 'deepseek-v4.1-flash',
          priority: 501,
          weight: 100,
        },
      ],
    },
  ])
  show()

  await userEvent.click(
    await screen.findByRole('button', { name: '全部 ollama' })
  )
  await userEvent.click(await screen.findByRole('button', { name: 'Apply' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.weights'])).toEqual([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 501, weight: 100 },
  ])
})

it('marks the matching preset as active', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500, weight: 50 },
  ])
  presetsRaw = JSON.stringify([
    {
      name: '默认',
      weights: [
        {
          channel_id: 9,
          model: 'deepseek-v4.1-flash',
          priority: 500,
          weight: 50,
        },
      ],
    },
  ])
  show()

  const preset = await screen.findByRole('button', { name: '默认' })
  await waitFor(() => expect(preset).toHaveAttribute('aria-pressed', 'true'))
})

it('saves the current rows as a named preset', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
  ])
  show()

  await screen.findByText('#9 - channel-nine')
  await userEvent.click(screen.getByRole('button', { name: 'Manage presets' }))
  await userEvent.type(
    await screen.findByRole('textbox', { name: 'Preset name' }),
    '默认'
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Save current as preset' })
  )

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.presets'])).toEqual([
    {
      name: '默认',
      weights: [{ channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 }],
    },
  ])
})

it('deletes a preset after confirmation', async () => {
  weights = '[]'
  presetsRaw = JSON.stringify([
    { name: '默认', weights: [{ channel_id: 9, model: 'm', weight: 1 }] },
    { name: '全部 ollama', weights: [] },
  ])
  show()

  await userEvent.click(screen.getByRole('button', { name: 'Manage presets' }))
  const removeButtons = await screen.findAllByRole('button', { name: 'Remove' })
  await userEvent.click(removeButtons[0])
  await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.presets'])).toEqual([
    { name: '全部 ollama', weights: [] },
  ])
})

it('renders one ratio selector per model group with an accessible model name', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 700 },
    { channel_id: 36, model: 'glm-4.6', weight: 300 },
  ])
  show()

  expect(await screen.findByText('deepseek-v4.1-flash')).toBeVisible()
  expect(screen.getByText('glm-4.6')).toBeVisible()
  expect(
    screen.getByRole('combobox', { name: 'Routing ratio deepseek-v4.1-flash' })
  ).toBeVisible()
  expect(
    screen.getByRole('combobox', { name: 'Routing ratio glm-4.6' })
  ).toBeVisible()
})

it('applies a scoped preset to one model and leaves the other model untouched', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500, weight: 700 },
    { channel_id: 36, model: 'glm-4.6', priority: 400, weight: 300 },
  ])
  presetsRaw = JSON.stringify([
    {
      name: 'ollama',
      model: 'deepseek-v4.1-flash',
      weights: [
        {
          channel_id: 9,
          model: 'deepseek-v4.1-flash',
          priority: 501,
          weight: 100,
        },
      ],
    },
  ])
  show()

  const select = await screen.findByRole('combobox', {
    name: 'Routing ratio deepseek-v4.1-flash',
  })
  await userEvent.click(select)
  await userEvent.click(await screen.findByRole('option', { name: 'ollama' }))
  await userEvent.click(await screen.findByRole('button', { name: 'Apply' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(payload).toEqual({
    options: {
      'model_weight_setting.weights': JSON.stringify([
        {
          channel_id: 9,
          model: 'deepseek-v4.1-flash',
          weight: 100,
          priority: 501,
        },
        {
          channel_id: 36,
          model: 'glm-4.6',
          weight: 300,
          priority: 400,
        },
      ]),
    },
  })
})

it('opens the custom ratio dialog and cancels without patching', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500, weight: 700 },
    {
      channel_id: 36,
      model: 'deepseek-v4.1-flash',
      priority: 500,
      weight: 300,
    },
  ])
  show()

  const select = await screen.findByRole('combobox', {
    name: 'Routing ratio deepseek-v4.1-flash',
  })
  await userEvent.click(select)
  await userEvent.click(
    await screen.findByRole('option', { name: 'Custom ratio' })
  )

  expect(
    await screen.findByText(
      'Set the ratio as percentages. They are stored as integer weights.'
    )
  ).toBeVisible()
  expect(api.patch).not.toHaveBeenCalled()

  await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
  await waitFor(() =>
    expect(
      screen.queryByText(
        'Set the ratio as percentages. They are stored as integer weights.'
      )
    ).not.toBeInTheDocument()
  )
  expect(api.patch).not.toHaveBeenCalled()
})

it('writes normalized integer weights from the custom ratio dialog', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500, weight: 700 },
    {
      channel_id: 36,
      model: 'deepseek-v4.1-flash',
      priority: 500,
      weight: 300,
    },
  ])
  show()

  const select = await screen.findByRole('combobox', {
    name: 'Routing ratio deepseek-v4.1-flash',
  })
  await userEvent.click(select)
  await userEvent.click(
    await screen.findByRole('option', { name: 'Custom ratio' })
  )

  const first = await screen.findByRole('spinbutton', { name: 'Share #9' })
  const second = await screen.findByRole('spinbutton', { name: 'Share #36' })
  fireEvent.change(first, { target: { value: '0.4' } })
  fireEvent.change(second, { target: { value: '99.6' } })
  await userEvent.click(screen.getByRole('button', { name: 'Apply' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.weights'])).toEqual([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 1, priority: 500 },
    {
      channel_id: 36,
      model: 'deepseek-v4.1-flash',
      weight: 100,
      priority: 500,
    },
  ])
})

it('shows each row share within its own priority tier', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500, weight: 700 },
    {
      channel_id: 36,
      model: 'deepseek-v4.1-flash',
      priority: 500,
      weight: 300,
    },
    {
      channel_id: 46,
      model: 'deepseek-v4.1-flash',
      priority: 400,
      weight: 100,
    },
    {
      channel_id: 47,
      model: 'deepseek-v4.1-flash',
      priority: 400,
      weight: 300,
    },
  ])
  show()

  expect(await screen.findByText('70%')).toBeVisible()
  expect(screen.getByText('30%')).toBeVisible()
  expect(screen.getByText('25%')).toBeVisible()
  expect(screen.getByText('75%')).toBeVisible()
})

it('shows a dash instead of a share when the tier sums to zero', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500, weight: 0 },
    { channel_id: 36, model: 'deepseek-v4.1-flash', priority: 500, weight: 0 },
  ])
  show()

  expect(await screen.findAllByText('-')).toHaveLength(2)
})

it('shows a dash and a hint when the channel list fails and no weight is set', async () => {
  channelState.error = new Error('boom')
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500 },
  ])
  show()

  expect(await screen.findByText('deepseek-v4.1-flash')).toBeVisible()
  expect(await screen.findAllByText('-')).not.toHaveLength(0)
  expect(
    screen.getByText('The share is computed inside one priority tier.')
  ).toBeVisible()
})

it('marks a group as unsaved while the ratio selector keeps the saved preset', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 500, weight: 700 },
  ])
  presetsRaw = JSON.stringify([
    {
      name: 'ollama',
      model: 'deepseek-v4.1-flash',
      weights: [
        {
          channel_id: 9,
          model: 'deepseek-v4.1-flash',
          priority: 500,
          weight: 700,
        },
      ],
    },
  ])
  show()

  const select = await screen.findByRole('combobox', {
    name: 'Routing ratio deepseek-v4.1-flash',
  })
  expect(select).toHaveTextContent('ollama')

  fireEvent.change(screen.getByRole('spinbutton', { name: 'Weight' }), {
    target: { value: '5' },
  })

  expect(await screen.findByText('Unsaved changes')).toBeVisible()
  expect(select).toHaveTextContent('ollama')
})

it('lists only global presets in the quick-switch bar', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
  ])
  presetsRaw = JSON.stringify([
    { name: 'global-one', weights: [] },
    { name: 'scoped-one', model: 'deepseek-v4.1-flash', weights: [] },
  ])
  show()

  expect(
    await screen.findByRole('button', { name: 'global-one' })
  ).toBeVisible()
  expect(
    screen.queryByRole('button', { name: 'scoped-one' })
  ).not.toBeInTheDocument()
})

it('saves a scoped preset with only the chosen model rows', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
    { channel_id: 36, model: 'glm-4.6', weight: 5 },
  ])
  show()

  await screen.findByText('#9 - channel-nine')
  await userEvent.click(screen.getByRole('button', { name: 'Manage presets' }))
  await userEvent.type(
    await screen.findByRole('textbox', { name: 'Preset name' }),
    'scoped'
  )
  await userEvent.click(screen.getByRole('combobox', { name: 'Preset scope' }))
  await userEvent.click(
    await screen.findByRole('option', { name: 'Only deepseek-v4.1-flash' })
  )
  await userEvent.click(
    screen.getByRole('button', { name: 'Save current as preset' })
  )

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.presets'])).toEqual([
    {
      name: 'scoped',
      model: 'deepseek-v4.1-flash',
      weights: [{ channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 }],
    },
  ])
})

it('deletes a preset by scope and name', async () => {
  weights = '[]'
  presetsRaw = JSON.stringify([
    { name: 'dup', weights: [] },
    { name: 'dup', model: 'deepseek-v4.1-flash', weights: [] },
  ])
  show()

  await userEvent.click(screen.getByRole('button', { name: 'Manage presets' }))
  const removeButtons = await screen.findAllByRole('button', { name: 'Remove' })
  await userEvent.click(removeButtons[1])
  await userEvent.click(await screen.findByRole('button', { name: 'Delete' }))

  await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
  const payload = vi.mocked(api.patch).mock.calls[0][1] as {
    options: Record<string, string>
  }
  expect(JSON.parse(payload.options['model_weight_setting.presets'])).toEqual([
    { name: 'dup', weights: [] },
  ])
})

it('snapshots the current editor rows when saving a preset', async () => {
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
  ])
  show()

  await screen.findByText('#9 - channel-nine')
  fireEvent.change(screen.getByRole('spinbutton', { name: 'Weight' }), {
    target: { value: '30' },
  })
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
  expect(JSON.parse(payload.options['model_weight_setting.presets'])).toEqual([
    {
      name: 'edited',
      weights: [{ channel_id: 9, model: 'deepseek-v4.1-flash', weight: 30 }],
    },
  ])
})

describe('custom ratio locked tiers', () => {
  const channel = (id: number, priority: number, weight: number) =>
    ({ id, name: `channel-${id}`, priority, weight }) as Channel

  const channelById = new Map<number, Channel>([
    [9, channel(9, 500, 100)],
    [36, channel(36, 500, 100)],
  ])

  const mixedWeights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 700, priority: 500 },
    {
      channel_id: 36,
      model: 'deepseek-v4.1-flash',
      weight: 300,
      priority: 500,
    },
    {
      channel_id: 47,
      model: 'deepseek-v4.1-flash',
      weight: 100,
      priority: 400,
    },
    { channel_id: 48, model: 'deepseek-v4.1-flash', priority: 400 },
  ])

  it('marks a tier whose effective weights cannot all be resolved as locked without zeroing it', () => {
    const state = buildCustomRatioState(
      mixedWeights,
      'deepseek-v4.1-flash',
      channelById
    )
    expect(state.tiers.find((tier) => tier.tier === 500)?.locked).toBe(false)
    const locked = state.tiers.find((tier) => tier.tier === 400)
    expect(locked?.locked).toBe(true)
    for (const entry of locked?.entries ?? []) {
      expect(
        state.percents[`${entry.channel_id}|deepseek-v4.1-flash`]
      ).toBeUndefined()
    }
  })

  it('leaves locked tier entries byte-identical when applying editable percentages', () => {
    const state = buildCustomRatioState(
      mixedWeights,
      'deepseek-v4.1-flash',
      channelById
    )
    const result = applyCustomRatioWeights(mixedWeights, state)
    const inputLocked = (
      JSON.parse(mixedWeights) as ModelWeightOverrideEntry[]
    ).filter((entry) => entry.priority === 400)
    const outputLocked = (
      JSON.parse(result) as ModelWeightOverrideEntry[]
    ).filter((entry) => entry.priority === 400)
    expect(JSON.stringify(outputLocked)).toBe(JSON.stringify(inputLocked))
    expect(outputLocked).toEqual(inputLocked)
    expect('weight' in outputLocked[1]).toBe(false)
  })

  it('keeps the locked tier read-only and writes nothing for it', async () => {
    weights = mixedWeights
    show()

    const select = await screen.findByRole('combobox', {
      name: 'Routing ratio deepseek-v4.1-flash',
    })
    await userEvent.click(select)
    await userEvent.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )

    expect(
      await screen.findByText(
        'Channel data is unavailable, so this tier keeps its current weights.'
      )
    ).toBeVisible()
    const lockedShare = screen.getByRole('spinbutton', { name: 'Share #47' })
    expect(lockedShare).toBeDisabled()
    expect(lockedShare).toHaveAttribute('readonly')
    expect(screen.getByRole('spinbutton', { name: 'Share #9' })).toBeEnabled()

    await userEvent.click(screen.getByRole('button', { name: 'Apply' }))

    await waitFor(() => expect(api.patch).toHaveBeenCalledTimes(1))
    const payload = vi.mocked(api.patch).mock.calls[0][1] as {
      options: Record<string, string>
    }
    expect(JSON.parse(payload.options['model_weight_setting.weights'])).toEqual(
      [
        {
          channel_id: 9,
          model: 'deepseek-v4.1-flash',
          weight: 70,
          priority: 500,
        },
        {
          channel_id: 36,
          model: 'deepseek-v4.1-flash',
          weight: 30,
          priority: 500,
        },
        {
          channel_id: 47,
          model: 'deepseek-v4.1-flash',
          weight: 100,
          priority: 400,
        },
        { channel_id: 48, model: 'deepseek-v4.1-flash', priority: 400 },
      ]
    )
  })

  it('disables Apply and explains why when every tier is locked', async () => {
    weights = JSON.stringify([
      {
        channel_id: 47,
        model: 'deepseek-v4.1-flash',
        weight: 100,
        priority: 400,
      },
      { channel_id: 48, model: 'deepseek-v4.1-flash', priority: 400 },
    ])
    show()

    const select = await screen.findByRole('combobox', {
      name: 'Routing ratio deepseek-v4.1-flash',
    })
    await userEvent.click(select)
    await userEvent.click(
      await screen.findByRole('option', { name: 'Custom ratio' })
    )

    expect(
      await screen.findByText(
        'Channel data is unavailable, so this tier keeps its current weights.'
      )
    ).toBeVisible()
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled()
  })
})
