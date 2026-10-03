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
import { useQuery } from '@tanstack/react-query'

import { mobileApiGet } from '@/mobile/lib/api-client'
import { MOBILE_STALE_TIME } from '@/mobile/lib/query-client'
import type {
  LogStat,
  PagedResult,
  QuotaDataRow,
  TimeRange,
  UsageLogRow,
  UsageScope,
} from '@/mobile/types'

export function useLiveRate(scope: UsageScope, range: TimeRange) {
  return useQuery({
    queryKey: ['mobile', 'usage', 'stat', scope, range.start, range.end],
    queryFn: () =>
      mobileApiGet<LogStat>(
        scope === 'all' ? '/api/log/stat' : '/api/log/self/stat',
        {
          start_timestamp: range.start,
          end_timestamp: range.end,
        }
      ),
    staleTime: MOBILE_STALE_TIME.usage,
  })
}

export function useUsageAggregate(scope: UsageScope, range: TimeRange) {
  return useQuery({
    queryKey: ['mobile', 'usage', 'aggregate', scope, range.start, range.end],
    queryFn: () =>
      mobileApiGet<QuotaDataRow[]>(
        scope === 'all' ? '/api/data/' : '/api/data/self',
        {
          start_timestamp: range.start,
          end_timestamp: range.end,
        }
      ),
    staleTime: MOBILE_STALE_TIME.usage,
  })
}

export function useUsageRanking(
  scope: UsageScope,
  range: TimeRange,
  enabled: boolean
) {
  return useQuery({
    queryKey: ['mobile', 'usage', 'ranking', range.start, range.end],
    queryFn: () =>
      mobileApiGet<QuotaDataRow[]>('/api/data/users', {
        start_timestamp: range.start,
        end_timestamp: range.end,
      }),
    staleTime: MOBILE_STALE_TIME.usage,
    enabled: scope === 'all' && enabled,
  })
}

export function useRecentLogs(
  scope: UsageScope,
  range: TimeRange,
  type: number
) {
  return useQuery({
    queryKey: ['mobile', 'usage', 'logs', scope, range.start, range.end, type],
    queryFn: () =>
      mobileApiGet<PagedResult<UsageLogRow>>(
        scope === 'all' ? '/api/log' : '/api/log/self',
        {
          p: 1,
          page_size: 20,
          type,
          start_timestamp: range.start,
          end_timestamp: range.end,
        }
      ),
    staleTime: MOBILE_STALE_TIME.usage,
  })
}
