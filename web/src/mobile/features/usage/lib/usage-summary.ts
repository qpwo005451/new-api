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
import dayjs from '@/lib/dayjs'
import type {
  QuotaDataRow,
  RankRow,
  TimeRange,
  TimeRangePreset,
  UsageTotals,
} from '@/mobile/types'

const PRESET_DAYS: Record<Exclude<TimeRangePreset, 'today'>, number> = {
  '7d': 7,
  '30d': 30,
}

export function resolveTimeRange(
  preset: TimeRangePreset,
  now: number
): TimeRange {
  if (preset === 'today') {
    return { start: dayjs.unix(now).startOf('day').unix(), end: now }
  }
  return { start: now - PRESET_DAYS[preset] * 24 * 60 * 60, end: now }
}

export function aggregateTotals(rows: QuotaDataRow[]): UsageTotals {
  return rows.reduce<UsageTotals>(
    (totals, row) => ({
      requests: totals.requests + row.count,
      tokens: totals.tokens + row.token_used,
      quota: totals.quota + row.quota,
    }),
    { requests: 0, tokens: 0, quota: 0 }
  )
}

export function rankRows(
  rows: QuotaDataRow[],
  key: 'model_name' | 'username',
  limit: number
): RankRow[] {
  const grouped = new Map<string, RankRow>()
  for (const row of rows) {
    const name = (key === 'model_name' ? row.model_name : row.username) ?? ''
    if (name === '') {
      continue
    }
    const current = grouped.get(name) ?? {
      key: name,
      requests: 0,
      tokens: 0,
      quota: 0,
    }
    current.requests += row.count
    current.tokens += row.token_used
    current.quota += row.quota
    grouped.set(name, current)
  }
  return [...grouped.values()]
    .sort((left, right) => right.quota - left.quota)
    .slice(0, limit)
}
