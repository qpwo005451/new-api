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
import { describe, expect, it } from 'vitest'

import { aggregateTotals, rankRows, resolveTimeRange } from '@/mobile/features/usage/lib/usage-summary'
import type { QuotaDataRow } from '@/mobile/types'

const rows: QuotaDataRow[] = [
  { model_name: 'gpt-5', created_at: 1, count: 10, quota: 500, token_used: 1000 },
  { model_name: 'gpt-5', created_at: 2, count: 5, quota: 250, token_used: 500 },
  { model_name: 'claude-5', created_at: 1, count: 1, quota: 900, token_used: 30 },
]

describe('resolveTimeRange', () => {
  // 2026-10-05T12:00:00Z
  const now = 1775476800

  it('starts today at the local midnight of the current day', () => {
    const range = resolveTimeRange('today', now)

    expect(range.end).toBe(now)
    expect(new Date(range.start * 1000).getHours()).toBe(0)
    expect(range.start).toBeLessThanOrEqual(now)
  })

  it('covers seven days for the 7d preset', () => {
    const range = resolveTimeRange('7d', now)

    expect(range.end - range.start).toBe(7 * 24 * 60 * 60)
  })

  it('covers thirty days for the 30d preset', () => {
    const range = resolveTimeRange('30d', now)

    expect(range.end - range.start).toBe(30 * 24 * 60 * 60)
  })
})

describe('aggregateTotals', () => {
  it('sums requests, tokens and quota', () => {
    expect(aggregateTotals(rows)).toEqual({ requests: 16, tokens: 1530, quota: 1650 })
  })

  it('returns zeros for an empty range', () => {
    expect(aggregateTotals([])).toEqual({ requests: 0, tokens: 0, quota: 0 })
  })
})

describe('rankRows', () => {
  it('groups by model, sums, sorts by quota and truncates', () => {
    // Quota-descending, mirroring the desktop user ranking (processUserChartData
    // sorts `b - a`), so claude-5 (900) outranks gpt-5 (750). The brief listed
    // gpt-5 first, which contradicted both its own rankRows sort and the
    // desktop order; the expectation was corrected to match the real behaviour.
    expect(rankRows(rows, 'model_name', 2)).toEqual([
      { key: 'claude-5', requests: 1, tokens: 30, quota: 900 },
      { key: 'gpt-5', requests: 15, tokens: 1500, quota: 750 },
    ])
  })

  it('returns an empty list when there are no rows', () => {
    expect(rankRows([], 'model_name', 5)).toEqual([])
  })

  it('keeps a single row unchanged', () => {
    expect(rankRows([rows[2]], 'model_name', 5)).toEqual([
      { key: 'claude-5', requests: 1, tokens: 30, quota: 900 },
    ])
  })
})
