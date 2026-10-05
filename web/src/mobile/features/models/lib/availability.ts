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
import type {
  AvailabilityHealth,
  AvailabilityRow,
  MonitorSiteResponse,
} from '@/mobile/types'

const HEALTH_BUCKETS: readonly AvailabilityHealth[] = [
  'normal',
  'degraded',
  'unavailable',
  'unknown',
]

function normalizeHealth(value: string): AvailabilityHealth {
  return HEALTH_BUCKETS.find((health) => health === value) ?? 'unknown'
}

export function countByHealth(
  sites: MonitorSiteResponse[]
): Record<AvailabilityHealth, number> {
  const counts: Record<AvailabilityHealth, number> = {
    normal: 0,
    degraded: 0,
    unavailable: 0,
    unknown: 0,
  }
  for (const site of sites) {
    counts[normalizeHealth(site.summary.health)] += 1
  }
  return counts
}

export function flattenAvailability(
  sites: MonitorSiteResponse[]
): AvailabilityRow[] {
  const rows: AvailabilityRow[] = []
  for (const site of sites) {
    for (const model of site.summary.models) {
      rows.push({
        siteName: site.site.name,
        modelName: model.model_name,
        status: model.status,
        latestFailureType: model.latest_failure_type,
        latestErrorSummary: model.latest_error_summary,
        stale: model.stale,
      })
    }
  }
  const severity = (row: AvailabilityRow): number => {
    if (row.status === 'unavailable') {
      return 0
    }
    if (row.status === 'limited') {
      return 1
    }
    if (row.stale) {
      return 2
    }
    return 3
  }
  return rows.sort(
    (left, right) =>
      severity(left) - severity(right) ||
      left.modelName.localeCompare(right.modelName)
  )
}
