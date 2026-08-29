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
import type { CellContext } from '@tanstack/react-table'
import { render } from '@testing-library/react'
import i18next from 'i18next'
import { beforeAll, describe, expect, test, vi } from 'vitest'

import type { UsageLog } from '../../data/schema'
import { useCommonLogsColumns } from '../columns/common-logs-columns'

// @lobehub/icons transitively hits a broken ESM directory import that fails
// under vitest; the client info columns under test never render model icons.
vi.mock('@/lib/lobe-icon', () => ({ getLobeIcon: () => null }))

beforeAll(() => {
  i18next.addResourceBundle('en', 'translation', {
    IP: 'IP',
    'User Agent': 'User Agent',
  })
})

function createUsageLog(other: string, ip = ''): UsageLog {
  return {
    id: 1,
    user_id: 1,
    created_at: 1,
    type: 2,
    content: 'done',
    username: 'tester',
    token_name: 'tok',
    model_name: 'deepseek-v4-flash',
    quota: 100,
    prompt_tokens: 10,
    completion_tokens: 5,
    use_time: 3,
    is_stream: false,
    channel: 9,
    channel_name: 'input',
    token_id: 1,
    group: 'default',
    ip,
    other,
    request_id: 'request-1',
    upstream_request_id: '',
  }
}

function ClientInfoCell(props: {
  isAdmin: boolean
  columnId: string
  log: UsageLog
}) {
  const columns = useCommonLogsColumns(props.isAdmin, {
    onViewDetails() {},
    onCancelRequest() {},
  })
  const column = columns.find((item) => item.id === props.columnId)
  if (!column || typeof column.cell !== 'function') return null
  return column.cell({
    row: { original: props.log },
  } as CellContext<UsageLog, unknown>)
}

function renderCell(props: {
  isAdmin: boolean
  columnId: string
  log: UsageLog
}): string {
  const rendered = render(<ClientInfoCell {...props} />)
  return rendered.container.textContent ?? ''
}

function ColumnsProbe(props: { isAdmin: boolean }) {
  const columns = useCommonLogsColumns(props.isAdmin, {
    onViewDetails() {},
    onCancelRequest() {},
  })
  const ids = columns.map(
    (column) =>
      column.id ?? (column as { accessorKey?: string }).accessorKey ?? ''
  )
  return <div data-testid='column-ids'>{ids.join(',')}</div>
}

function renderColumnIds(isAdmin: boolean): string[] {
  const rendered = render(<ColumnsProbe isAdmin={isAdmin} />)
  return (rendered.getByTestId('column-ids').textContent ?? '').split(',')
}

function columnIdSet(isAdmin: boolean): Set<string> {
  return new Set(renderColumnIds(isAdmin))
}

describe('common logs client info columns', () => {
  test('shows the admin-only IP and User-Agent columns for admins', () => {
    const log = createUsageLog(
      JSON.stringify({
        admin_info: { ip: '203.0.113.7', user_agent: 'Go-http-client/1.1' },
      })
    )

    const adminColumns = columnIdSet(true)
    expect(adminColumns.has('client_ip')).toBe(true)
    expect(adminColumns.has('user_agent')).toBe(true)

    expect(renderCell({ isAdmin: true, columnId: 'client_ip', log })).toContain(
      '203.0.113.7'
    )
    expect(
      renderCell({ isAdmin: true, columnId: 'user_agent', log })
    ).toContain('Go-http-client/1.1')
  })

  test('falls back to the user-visible log IP column', () => {
    const log = createUsageLog(JSON.stringify({}), '198.51.100.4')

    expect(renderCell({ isAdmin: true, columnId: 'client_ip', log })).toContain(
      '198.51.100.4'
    )
  })

  test('renders nothing when no client info is available', () => {
    const log = createUsageLog(JSON.stringify({}))

    expect(renderCell({ isAdmin: true, columnId: 'client_ip', log })).toBe('')
    expect(renderCell({ isAdmin: true, columnId: 'user_agent', log })).toBe('')
  })

  test('keeps the client info columns out of non-admin views', () => {
    const columns = columnIdSet(false)

    expect(columns.has('client_ip')).toBe(false)
    expect(columns.has('user_agent')).toBe(false)
  })
})
