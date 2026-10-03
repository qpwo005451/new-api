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
import type { RoutingModelChannelStat, RoutingSwitchStat } from '@/mobile/types'

export interface RoutingChannelShare {
  channelId: number
  channelName: string
  requests: number
  configuredShare: number | null
}

export interface RoutingShareRow {
  modelName: string
  requests: number
  errors: number
  avgUseTime: number
  configuredShare: number | null
  channels: RoutingChannelShare[]
}

export function summarizeRoutingShare(
  rows: RoutingModelChannelStat[]
): RoutingShareRow[] {
  const byModel = new Map<string, RoutingModelChannelStat[]>()
  for (const row of rows) {
    const bucket = byModel.get(row.model_name)
    if (bucket) {
      bucket.push(row)
    } else {
      byModel.set(row.model_name, [row])
    }
  }

  return [...byModel.entries()].map(([modelName, channels]) => {
    const requests = channels.reduce(
      (total, channel) => total + channel.requests,
      0
    )
    const configuredWeight = channels.reduce(
      (total, channel) => total + (channel.weight ?? 0),
      0
    )
    const hasConfiguredWeight = channels.some(
      (channel) => typeof channel.weight === 'number' && channel.weight > 0
    )
    return {
      modelName,
      requests,
      errors: channels.reduce((total, channel) => total + channel.errors, 0),
      avgUseTime:
        channels.reduce(
          (total, channel) => total + channel.avg_use_time * channel.requests,
          0
        ) / (requests || 1),
      configuredShare: hasConfiguredWeight ? 1 : null,
      channels: channels
        .map((channel) => ({
          channelId: channel.channel_id,
          channelName: channel.channel_name,
          requests: channel.requests,
          configuredShare: hasConfiguredWeight
            ? (channel.weight ?? 0) / configuredWeight
            : null,
        }))
        .sort((left, right) => right.requests - left.requests),
    }
  })
}

export function topSwitches(
  rows: RoutingSwitchStat[],
  limit: number
): RoutingSwitchStat[] {
  return [...rows]
    .sort((left, right) => right.count - left.count)
    .slice(0, limit)
}
