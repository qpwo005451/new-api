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
import { keepPreviousData, useQuery } from '@tanstack/react-query'

import { mobileApiGet } from '@/mobile/lib/api-client'
import { MOBILE_STALE_TIME } from '@/mobile/lib/query-client'
import type {
  ChannelAffinityBindings,
  RoutingStats,
  TimeRange,
} from '@/mobile/types'

export function useRoutingStats(range: TimeRange) {
  return useQuery({
    queryKey: ['mobile', 'routing', 'stats', range.start, range.end],
    queryFn: () =>
      mobileApiGet<RoutingStats>('/api/log/routing_stats', {
        start_timestamp: range.start,
        end_timestamp: range.end,
      }),
    staleTime: MOBILE_STALE_TIME.routing,
    placeholderData: keepPreviousData,
  })
}

export function useAffinityBindings() {
  return useQuery({
    queryKey: ['mobile', 'routing', 'affinity'],
    queryFn: () =>
      mobileApiGet<ChannelAffinityBindings>(
        '/api/log/channel_affinity_bindings',
        { limit: 20 }
      ),
    staleTime: MOBILE_STALE_TIME.routing,
  })
}
