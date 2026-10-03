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

import { countByHealth, flattenAvailability } from '@/mobile/features/models/lib/availability'
import type { MonitorSiteResponse } from '@/mobile/types'

const sites: MonitorSiteResponse[] = [
  {
    site: { id: 1, name: 'primary', enabled: true },
    summary: {
      score: 80,
      health: 'degraded',
      models: [
        { model_name: 'gpt-5', status: 'available', latest_status: 'success', weight: 100, stale: false },
        {
          model_name: 'claude-5',
          status: 'unavailable',
          latest_status: 'failure',
          latest_failure_type: 'timeout',
          latest_error_summary: 'upstream timeout',
          weight: 50,
          stale: false,
        },
      ],
    },
    channel_ids: [3],
    latest_observed_at: 1000,
    freshness_seconds: 120,
  },
  {
    site: { id: 2, name: 'backup', enabled: true },
    summary: { score: 100, health: 'normal', models: [] },
    channel_ids: [],
    latest_observed_at: 0,
    freshness_seconds: undefined,
  },
]

describe('countByHealth', () => {
  it('counts sites per health bucket including unknown', () => {
    expect(countByHealth(sites)).toEqual({ normal: 1, degraded: 1, unavailable: 0, unknown: 0 })
  })

  it('returns zeros for no sites', () => {
    expect(countByHealth([])).toEqual({ normal: 0, degraded: 0, unavailable: 0, unknown: 0 })
  })
})

describe('flattenAvailability', () => {
  it('lists every model with its site and puts problems first', () => {
    const rows = flattenAvailability(sites)

    expect(rows.map((row) => row.modelName)).toEqual(['claude-5', 'gpt-5'])
    expect(rows[0]).toMatchObject({ siteName: 'primary', status: 'unavailable', latestFailureType: 'timeout' })
    expect(rows[1]).toMatchObject({ siteName: 'primary', status: 'available' })
  })

  it('returns an empty list when no site exposes models', () => {
    expect(flattenAvailability([sites[1]])).toEqual([])
  })
})
