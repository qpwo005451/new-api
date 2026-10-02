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

import type { ChannelAffinityBinding, RoutingTrendPoint } from './types'

export type RoutingStatsWindowPreset = '1h' | '24h' | '7d'

const PRESET_SECONDS: Record<RoutingStatsWindowPreset, number> = {
  '1h': 3600,
  '24h': 24 * 3600,
  '7d': 7 * 24 * 3600,
}

/**
 * Builds the query window for a preset. The end carries a small buffer so rows
 * written while the request is in flight still fall inside the window.
 */
export function routingStatsTimeRange(
  preset: RoutingStatsWindowPreset,
  now: number = Math.floor(Date.now() / 1000)
): { start_timestamp: number; end_timestamp: number } {
  return {
    start_timestamp: now - PRESET_SECONDS[preset],
    end_timestamp: now + 60,
  }
}

export function routingChannelKey(
  modelName: string,
  channelID: number
): string {
  return `${modelName}\u0000${channelID}`
}

export function routingChannelLabel(
  channelID: number,
  channelName: string
): string {
  return channelName || `#${channelID}`
}

export interface RoutingShareInput {
  model_name: string
  channel_id: number
  requests: number
}

export interface RoutingConfiguredInput {
  model_name: string
  channel_id: number
  priority?: number
  weight?: number
}

/**
 * Mirrors the selection rule the gateway actually applies: only the highest
 * priority tier competes for traffic, and requests split by weight inside that
 * tier. Channels in a lower tier are retry backups, so their configured share
 * is zero. When no channel in the top tier carries a weight, selection falls
 * back to an equal split.
 *
 * Rows whose channel could not be read have no effective priority, so they are
 * left out of the result.
 */
export function configuredShares(
  rows: RoutingConfiguredInput[]
): Map<string, number> {
  const byModel = new Map<string, RoutingConfiguredInput[]>()
  for (const row of rows) {
    const list = byModel.get(row.model_name)
    if (list) list.push(row)
    else byModel.set(row.model_name, [row])
  }

  const shares = new Map<string, number>()
  for (const [modelName, modelRows] of byModel) {
    const priorities = modelRows
      .map((row) => row.priority)
      .filter((priority): priority is number => typeof priority === 'number')
    if (priorities.length === 0) continue

    const topPriority = Math.max(...priorities)
    const tier = modelRows.filter((row) => row.priority === topPriority)
    const totalWeight = tier.reduce((sum, row) => sum + (row.weight ?? 0), 0)

    for (const row of modelRows) {
      const key = routingChannelKey(modelName, row.channel_id)
      if (row.priority !== topPriority) {
        shares.set(key, 0)
        continue
      }
      shares.set(
        key,
        totalWeight > 0 ? (row.weight ?? 0) / totalWeight : 1 / tier.length
      )
    }
  }
  return shares
}

/** Share of a model's requests, or sessions, handled by each channel. */
export function actualShares(rows: RoutingShareInput[]): Map<string, number> {
  const totals = new Map<string, number>()
  for (const row of rows) {
    totals.set(row.model_name, (totals.get(row.model_name) ?? 0) + row.requests)
  }

  const shares = new Map<string, number>()
  for (const row of rows) {
    const total = totals.get(row.model_name) ?? 0
    shares.set(
      routingChannelKey(row.model_name, row.channel_id),
      total > 0 ? row.requests / total : 0
    )
  }
  return shares
}

export interface RoutingTrendSeries {
  time: string
  requests: number
  switched: number
}

/**
 * Labels the trend buckets. Minute granularity is kept below one hour per
 * bucket, because wider buckets only ever start on the hour.
 */
export function buildTrendSeries(
  points: RoutingTrendPoint[],
  bucketSeconds: number
): RoutingTrendSeries[] {
  const format = bucketSeconds < 3600 ? 'MM-DD HH:mm' : 'MM-DD HH:00'
  return points.map((point) => ({
    time: dayjs(point.timestamp * 1000).format(format),
    requests: point.requests,
    switched: point.switched,
  }))
}

export interface LiveAllocationRow {
  modelName: string
  channelID: number
  channelName: string
  sessions: number
}

/**
 * Groups the live session bindings by model and channel, which is the current
 * load balancing split for sticky sessions.
 */
export function buildLiveAllocation(
  entries: ChannelAffinityBinding[]
): LiveAllocationRow[] {
  const rows = new Map<string, LiveAllocationRow>()
  for (const entry of entries) {
    const key = routingChannelKey(entry.model_name, entry.channel_id)
    const row = rows.get(key)
    if (row) {
      row.sessions++
      continue
    }
    rows.set(key, {
      modelName: entry.model_name,
      channelID: entry.channel_id,
      channelName: entry.channel_name,
      sessions: 1,
    })
  }

  return [...rows.values()].sort(
    (left, right) =>
      left.modelName.localeCompare(right.modelName) ||
      right.sessions - left.sessions ||
      left.channelID - right.channelID
  )
}
