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
  createRoute,
  createRouter,
  RouterProvider,
} from '@tanstack/react-router'
import { act, cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, expect, test, vi } from 'vitest'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import { useSystemConfigStore } from '@/stores/system-config-store'

import { UsageLogsProvider } from '../usage-logs-provider'
import { UsageLogsTable } from '../usage-logs-table'

const clients: QueryClient[] = []

async function renderLogs(mobile = false, initiallyPending = false) {
  const matchMedia = window.matchMedia.bind(window)
  vi.spyOn(window, 'matchMedia').mockImplementation((query) => ({
    ...matchMedia(query),
    matches:
      query === '(max-width: 640px)' ? mobile : matchMedia(query).matches,
  }))
  useAuthStore.getState().auth.setUser({ id: 1, username: 'tester', role: 1 })
  let pending = initiallyPending
  let release: (() => void) | undefined
  vi.spyOn(api, 'get').mockImplementation(async (url) => {
    let data: unknown = {}
    if (url.startsWith('/api/log/self?')) {
      if (pending) {
        await new Promise<void>((resolve) => {
          release = resolve
        })
      }
      data = {
        items: [
          {
            id: 1,
            user_id: 1,
            created_at: 1788840000,
            type: 2,
            model_name: 'refresh-test-model',
            content: '',
            quota: 0,
            other: '{}',
          },
        ],
        total: 1,
      }
    } else if (url === '/api/subscription/self') {
      data = { subscriptions: [] }
    } else if (url.includes('/stat')) {
      data = { quota: 0, rpm: 0, tpm: 0 }
    }
    return { data: { success: true, data } }
  })
  const root = createRootRoute()
  const auth = createRoute({ getParentRoute: () => root, id: '_authenticated' })
  const logs = createRoute({
    getParentRoute: () => auth,
    path: '/usage-logs/$section',
    component: () => <UsageLogsTable logCategory='common' />,
    validateSearch: (search: Record<string, unknown>) => search,
  })
  const router = createRouter({
    routeTree: root.addChildren([auth.addChildren([logs])]),
    history: createMemoryHistory({ initialEntries: ['/usage-logs/common'] }),
  })
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  clients.push(client)
  const view = render(
    <QueryClientProvider client={client}>
      <UsageLogsProvider>
        <RouterProvider router={router} />
      </UsageLogsProvider>
    </QueryClientProvider>
  )
  await screen.findByRole('button', { name: 'Search' })
  if (!initiallyPending) {
    await screen.findAllByText('refresh-test-model')
    await waitFor(() => expect(client.isFetching()).toBe(0))
  }
  return {
    client,
    container: view.container,
    pause() {
      pending = true
    },
    async finish() {
      pending = false
      await act(async () => {
        release?.()
      })
      await waitFor(() => expect(client.isFetching()).toBe(0))
    },
  }
}

afterEach(() => {
  cleanup()
  for (const client of clients.splice(0)) client.clear()
  useAuthStore.setState(useAuthStore.getInitialState(), true)
  useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  localStorage.clear()
})

test.each([false, true])(
  'background refresh keeps existing logs fully opaque and interactive (mobile=%s)',
  async (mobile) => {
    const fixture = await renderLogs(mobile)
    for (let cycle = 0; cycle < 2; cycle += 1) {
      fixture.pause()
      act(() => {
        void fixture.client.refetchQueries({
          queryKey: ['logs'],
          type: 'active',
        })
      })
      await waitFor(() =>
        expect(fixture.client.isFetching({ queryKey: ['logs'] })).toBe(1)
      )
      expect(screen.getAllByText('refresh-test-model')[0]).toBeVisible()
      expect(fixture.container.querySelector('.opacity-60')).toBeNull()
      for (const model of screen.getAllByText('refresh-test-model')) {
        expect(model.closest('.pointer-events-none')).toBeNull()
      }
      await fixture.finish()
    }
  }
)

test('background refresh keeps Search enabled and out of its busy state', async () => {
  const fixture = await renderLogs()
  fixture.pause()
  act(() => {
    void fixture.client.refetchQueries({ queryKey: ['logs'], type: 'active' })
  })
  await waitFor(() =>
    expect(fixture.client.isFetching({ queryKey: ['logs'] })).toBe(1)
  )
  const search = screen.getByRole('button', { name: 'Search' })
  expect(search).toBeEnabled()
  expect(search).not.toHaveAttribute('aria-busy', 'true')
  expect(search.querySelector('.animate-spin')).toBeNull()
  await fixture.finish()
})

test('initial loading keeps the skeleton until logs arrive', async () => {
  const fixture = await renderLogs(false, true)
  await waitFor(() =>
    expect(fixture.client.isFetching({ queryKey: ['logs'] })).toBe(1)
  )
  expect(screen.queryByText('refresh-test-model')).not.toBeInTheDocument()
  expect(
    fixture.container.querySelector('[data-slot="skeleton"]')
  ).not.toBeNull()
  await fixture.finish()
  expect(screen.getAllByText('refresh-test-model')[0]).toBeVisible()
  expect(fixture.container.querySelector('[data-slot="skeleton"]')).toBeNull()
})

test('explicit Search stays busy until its log request completes', async () => {
  const fixture = await renderLogs()
  fixture.pause()
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
  await waitFor(() =>
    expect(fixture.client.isFetching({ queryKey: ['logs'] })).toBe(1)
  )
  expect(screen.getByRole('button', { name: 'Search' })).toBeDisabled()
  await fixture.finish()
  expect(screen.getByRole('button', { name: 'Search' })).toBeEnabled()
})
