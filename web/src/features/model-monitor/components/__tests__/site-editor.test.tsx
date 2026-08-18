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
*/
import assert from 'node:assert/strict'
import { after, before, describe, test } from 'node:test'

import { Window } from 'happy-dom'

import type { ModelMonitorSiteConfig } from '../../types'

const domWindow = new Window()
const domGlobals = [
  'window',
  'document',
  'navigator',
  'HTMLElement',
  'HTMLButtonElement',
  'HTMLInputElement',
  'SVGElement',
  'Node',
  'NodeFilter',
  'Element',
  'Event',
  'KeyboardEvent',
  'PointerEvent',
  'MouseEvent',
  'FocusEvent',
  'CustomEvent',
  'MutationObserver',
  'ResizeObserver',
  'requestAnimationFrame',
  'cancelAnimationFrame',
  'getComputedStyle',
] as const

for (const key of domGlobals) {
  Object.defineProperty(globalThis, key, {
    configurable: true,
    value: domWindow[key],
  })
}

const { act, useState } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { QueryClient, QueryClientProvider } =
  await import('@tanstack/react-query')
const { api } = await import('@/lib/api')
const { SiteEditor } = await import('../site-editor')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Channel IDs': 'Channel IDs',
        'Monitored models': 'Monitored models',
        'Select items...': 'Select items...',
        'No matching items': 'No matching items',
        'No channels found': 'No channels found',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

type ApiGet = (url: string, config?: unknown) => Promise<{ data: unknown }>

type ChannelFixture = {
  id: number
  name: string
}

const apiClient = api as unknown as { get: ApiGet }
const originalGet = apiClient.get
let channelRequestCount = 0
let selectedChannelUpdates: number[][] = []

function installChannelFixtures() {
  channelRequestCount = 0
  apiClient.get = async (url, config) => {
    channelRequestCount++
    assert.equal(url, '/api/channel')
    const params = (config as { params?: { p?: number } } | undefined)?.params
    if (params?.p === 1) {
      return {
        data: {
          success: true,
          data: {
            items: [
              { id: 11, name: 'primary' },
              { id: 27, name: 'reasoning' },
            ] satisfies ChannelFixture[],
            total: 101,
            page: 1,
            page_size: 100,
          },
        },
      }
    }
    return {
      data: {
        success: true,
        data: {
          items: [{ id: 31, name: 'fallback' } satisfies ChannelFixture],
          total: 101,
          page: 2,
          page_size: 100,
        },
      },
    }
  }
}

function createSite(): ModelMonitorSiteConfig {
  return {
    name: '66hxhx',
    site_type: 'newapi' as const,
    pricing_group: '',
    enabled: true,
    channel_ids: [11, 27],
    targets: [],
  }
}

function Harness() {
  const [site, setSite] = useState(createSite)
  const [queryClient] = useState(
    () =>
      new QueryClient({
        defaultOptions: { queries: { retry: false } },
      })
  )
  return (
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <SiteEditor
          site={site}
          onChange={(nextSite) => {
            selectedChannelUpdates.push(nextSite.channel_ids)
            setSite(nextSite)
          }}
          onRemove={() => undefined}
        />
        <output data-testid='channel-ids'>{site.channel_ids.join(',')}</output>
      </I18nextProvider>
    </QueryClientProvider>
  )
}

async function waitForCondition(
  condition: () => boolean,
  failureMessage: string
): Promise<void> {
  if (condition()) return

  await new Promise<void>((resolve, reject) => {
    const observer = new MutationObserver(() => {
      if (!condition()) return
      clearTimeout(timeoutId)
      observer.disconnect()
      resolve()
    })
    const timeoutId = setTimeout(() => {
      observer.disconnect()
      reject(new Error(failureMessage))
    }, 1500)
    observer.observe(document, {
      attributes: true,
      childList: true,
      characterData: true,
      subtree: true,
    })
  })
}

describe('model monitor site channel selector', () => {
  before(() => {
    installChannelFixtures()
  })

  after(() => {
    apiClient.get = originalGet
    domWindow.close()
  })

  test('keeps existing channel IDs while selecting another channel from the dropdown', async () => {
    selectedChannelUpdates = []
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)

    await act(async () => root.render(<Harness />))

    await act(async () => {
      await waitForCondition(
        () => channelRequestCount === 2,
        `channel options did not load all pages: requests=${channelRequestCount}; text=${container.textContent}`
      )
    })

    assert.deepEqual(selectedChannelUpdates, [])
    const channelInput = container.querySelector<HTMLInputElement>(
      'input[aria-label="Channel IDs"]'
    )
    assert.ok(channelInput)
    assert.equal(container.textContent?.includes('#11'), true)
    assert.equal(container.textContent?.includes('#27'), true)

    await act(async () => {
      channelInput.focus()
      channelInput.dispatchEvent(
        new domWindow.KeyboardEvent('keydown', {
          key: 'ArrowDown',
          bubbles: true,
        }) as unknown as Event
      )
    })
    await act(async () => {
      await waitForCondition(
        () => document.body.textContent?.includes('#11 - primary') === true,
        'channel options did not render'
      )
    })
    assert.deepEqual(selectedChannelUpdates, [])
    const fallbackOption = [
      ...document.querySelectorAll<HTMLElement>('[data-slot="combobox-item"]'),
    ].find((item) => item.textContent?.includes('31 - fallback'))
    assert.ok(fallbackOption)

    await act(async () => fallbackOption.click())

    assert.equal(
      container.querySelector('[data-testid="channel-ids"]')?.textContent,
      '11,27,31'
    )
    assert.deepEqual(selectedChannelUpdates, [[11, 27, 31]])

    await act(async () => root.unmount())
    container.remove()
  })
})
