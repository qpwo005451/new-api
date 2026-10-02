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
import { formatNumber, formatPercent } from '@/lib/format'

import {
  actualShares,
  buildLiveAllocation,
  configuredShares,
  routingChannelKey,
  routingChannelLabel,
  type LiveAllocationRow,
  type RoutingTrendSeries,
} from './lib'
import type {
  ChannelAffinityBinding,
  RoutingModelChannelStat,
  RoutingReasonStat,
} from './types'

/**
 * VChart spec builders for the routing page. They stay pure so the chart
 * contract can be tested without a canvas, and every builder returns null for
 * an empty data set so the caller can render an empty state instead.
 */
export type RoutingLabel = (key: string) => string

export function buildDistributionSpec(
  rows: RoutingModelChannelStat[],
  label: RoutingLabel
): Record<string, unknown> | null {
  if (rows.length === 0) return null

  const shares = actualShares(rows)
  const configured = configuredShares(rows)
  const values = rows.map((row) => {
    const key = routingChannelKey(row.model_name, row.channel_id)
    return {
      model: row.model_name,
      channel: routingChannelLabel(row.channel_id, row.channel_name),
      requests: row.requests,
      share: Number(((shares.get(key) ?? 0) * 100).toFixed(2)),
      configured: configured.has(key)
        ? formatPercent((configured.get(key) ?? 0) * 100)
        : '-',
    }
  })

  return {
    type: 'bar',
    data: [{ id: 'distribution', values }],
    xField: 'model',
    yField: 'share',
    seriesField: 'channel',
    stack: true,
    legends: { visible: true },
    axes: [
      {
        orient: 'left',
        title: { visible: true, text: label('Share of requests') },
        label: { formatMethod: (value: number) => formatPercent(value) },
      },
    ],
  }
}

export function buildTrendSpec(
  series: RoutingTrendSeries[],
  label: RoutingLabel
): Record<string, unknown> | null {
  if (series.length === 0) return null

  // VChart takes the legend name from series.name, so each series carries the
  // translated label and its own data view.
  const fields = [
    { field: 'requests', name: label('Requests') },
    { field: 'switched', name: label('Switched requests') },
  ]
  return {
    type: 'common',
    data: fields.map((item) => ({
      id: `trend-${item.field}`,
      values: series.map((row) => ({ ...row })),
    })),
    series: fields.map((item) => ({
      type: 'line',
      xField: 'time',
      yField: item.field,
      dataId: `trend-${item.field}`,
      name: item.name,
      point: { visible: false },
      line: { style: { lineWidth: 2 } },
    })),
    axes: [
      {
        orient: 'left',
        label: { formatMethod: (value: number) => formatNumber(value) },
      },
    ],
  }
}

export function buildReasonsSpec(
  reasons: RoutingReasonStat[],
  label: RoutingLabel
): Record<string, unknown> | null {
  if (reasons.length === 0) return null

  return {
    type: 'bar',
    data: [
      {
        id: 'reasons',
        values: reasons.map((reason) => ({
          reason: label(reason.reason),
          count: reason.count,
        })),
      },
    ],
    xField: 'reason',
    yField: 'count',
    seriesField: 'reason',
    legends: { visible: false },
    axes: [
      {
        orient: 'left',
        label: { formatMethod: (value: number) => formatNumber(value) },
      },
    ],
  }
}

export function buildLiveAllocationSpec(
  entries: ChannelAffinityBinding[],
  label: RoutingLabel
): Record<string, unknown> | null {
  const rows = buildLiveAllocation(entries)
  if (rows.length === 0) return null

  const shares = actualShares(
    rows.map((row: LiveAllocationRow) => ({
      model_name: row.modelName,
      channel_id: row.channelID,
      requests: row.sessions,
    }))
  )
  const values = rows.map((row) => ({
    model: row.modelName,
    channel: routingChannelLabel(row.channelID, row.channelName),
    sessions: row.sessions,
    share: Number(
      (
        (shares.get(routingChannelKey(row.modelName, row.channelID)) ?? 0) * 100
      ).toFixed(2)
    ),
  }))

  return {
    type: 'bar',
    data: [{ id: 'allocation', values }],
    xField: 'model',
    yField: 'share',
    seriesField: 'channel',
    stack: true,
    legends: { visible: true },
    axes: [
      {
        orient: 'left',
        title: { visible: true, text: label('Share of sessions') },
        label: { formatMethod: (value: number) => formatPercent(value) },
      },
    ],
  }
}
