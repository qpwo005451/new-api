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
import { afterEach, beforeEach, expect, it, vi } from 'vitest'

import { api } from '@/lib/api'

import { SettingsPageProvider } from '../../components/settings-page-context'
import { ModelWeightSection } from '../model-weight-section'

vi.mock('@/features/channels/api', () => ({
  getChannels: vi.fn(async () => ({
    success: true,
    data: {
      items: [
        { id: 9, name: 'channel-nine' },
        { id: 36, name: 'channel-thirty-six' },
      ],
      total: 2,
      page: 1,
      page_size: 100,
    },
  })),
}))

let client: QueryClient
let weights: string

beforeEach(() => {
  vi.spyOn(window, 'scrollTo').mockImplementation(() => undefined)
  client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  weights = JSON.stringify([
    { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 20 },
  ])
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
          defaultValues={{ 'model_weight_setting.weights': weights }}
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
  await userEvent.click(
    screen.getAllByRole('button', { name: 'Remove' })[0]
  )
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
  expect(JSON.parse(payload.options['model_weight_setting.weights'])).toEqual([])
})
