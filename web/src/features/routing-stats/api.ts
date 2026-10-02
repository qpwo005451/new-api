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
import { api } from '@/lib/api'

import type { ChannelAffinityBindings, RoutingStats } from './types'

export interface RoutingStatsParams {
  start_timestamp: number
  end_timestamp: number
  model_name?: string
  group?: string
  channel_id?: number
}

export async function getRoutingStats(
  params: RoutingStatsParams
): Promise<{ success: boolean; message?: string; data?: RoutingStats }> {
  const res = await api.get('/api/log/routing_stats', {
    params,
    disableDuplicate: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Reads the live session bindings. This is current state, not history, so it is
 * requested separately from the windowed routing statistics.
 */
export async function getChannelAffinityBindings(limit?: number): Promise<{
  success: boolean
  message?: string
  data?: ChannelAffinityBindings
}> {
  const res = await api.get('/api/log/channel_affinity_bindings', {
    params: limit === undefined ? undefined : { limit },
    disableDuplicate: true,
  } as Record<string, unknown>)
  return res.data
}
