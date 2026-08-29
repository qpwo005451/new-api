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
import assert from 'node:assert/strict'
import { describe, test, vi } from 'vitest'

import type { CellContext } from '@tanstack/react-table'

import type { UsageLog } from '../../data/schema'

// @lobehub/icons transitively hits a broken ESM directory import that fails
// under vitest; the reasoning effort column under test never renders model icons.
vi.mock('@/lib/lobe-icon', () => ({ getLobeIcon: () => null }))

const { act } = await import('react')
const { createRoot } = await import('react-dom/client')
const { createInstance } = await import('i18next')
const { I18nextProvider, initReactI18next } = await import('react-i18next')
const { useCommonLogsColumns } = await import('../columns/common-logs-columns')

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Reasoning Effort': 'Reasoning Effort',
        Time: 'Time',
        Token: 'Token',
        Model: 'Model',
        Stream: 'Stream',
        Tokens: 'Tokens',
        Cost: 'Cost',
        Timing: 'Timing',
        Details: 'Details',
      },
    },
  },
})

const reactTestGlobals = globalThis as typeof globalThis & {
  IS_REACT_ACT_ENVIRONMENT?: boolean
}
reactTestGlobals.IS_REACT_ACT_ENVIRONMENT = true

function createUsageLog(other: string): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 8,
    content: 'request in progress',
    username: '',
    token_name: '',
    model_name: 'gpt-5.6-sol',
    quota: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    use_time: 0,
    is_stream: true,
    channel: 9,
    channel_name: 'input',
    token_id: 1,
    group: 'default',
    ip: '',
    other,
    request_id: 'request-1',
    upstream_request_id: '',
  }
}

function ReasoningEffortColumn(props: { log: UsageLog }) {
  const columns = useCommonLogsColumns(false, {
    onViewDetails() {},
    onCancelRequest() {},
  })
  const column = columns.find((item) => item.id === 'reasoning_effort')
  if (!column || typeof column.cell !== 'function') return null

  return column.cell({
    row: {
      original: props.log,
    },
  } as CellContext<UsageLog, unknown>)
}

async function renderColumn(log: UsageLog): Promise<{
  container: HTMLDivElement
  root: ReturnType<typeof createRoot>
}> {
  const container = document.createElement('div')
  document.body.append(container)
  const root = createRoot(container)

  await act(async () => {
    root.render(
      <I18nextProvider i18n={i18n}>
        <ReasoningEffortColumn log={log} />
      </I18nextProvider>
    )
  })

  return { container, root }
}

async function unmount(rendered: {
  container: HTMLDivElement
  root: ReturnType<typeof createRoot>
}) {
  await act(async () => rendered.root.unmount())
  rendered.container.remove()
}

describe('common usage log reasoning effort column', () => {
  test('shows the requested reasoning effort for an in-progress response', async () => {
    const rendered = await renderColumn(
      createUsageLog('{"reasoning_effort":"high"}')
    )

    const badge = rendered.container.querySelector('[data-slot="status-badge"]')
    assert.ok(badge)
    assert.equal(badge.textContent, 'high')

    await unmount(rendered)
  })

  test('shows a placeholder when the request has no reasoning effort', async () => {
    const rendered = await renderColumn(createUsageLog('{}'))

    assert.equal(rendered.container.textContent, '-')

    await unmount(rendered)
  })
})
