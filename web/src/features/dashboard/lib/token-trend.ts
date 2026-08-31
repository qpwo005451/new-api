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
import type { TokenTrendPoint } from '@/features/dashboard/types'

export interface TokenTrendSeries {
  time: string
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
  cacheHitRate: number
}

// Reduce hourly buckets into chart rows. prompt_tokens already contains cache
// tokens, so plain input is the remainder after subtracting both cache series.
export function buildTokenTrendSeries(
  points: TokenTrendPoint[]
): TokenTrendSeries[] {
  return points.map((point) => {
    const cacheRead = point.cache_read
    const cacheWrite = point.cache_write
    const input = Math.max(point.prompt_tokens - cacheRead - cacheWrite, 0)
    const promptTotal = point.prompt_tokens
    return {
      time: formatBucketLabel(point.created_at),
      input,
      output: point.completion_tokens,
      cacheRead,
      cacheWrite,
      cacheHitRate:
        promptTotal > 0
          ? Number(((cacheRead / promptTotal) * 100).toFixed(1))
          : 0,
    }
  })
}

function formatBucketLabel(bucketTs: number): string {
  const date = new Date(bucketTs * 1000)
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  return `${month}-${day} ${hours}:00`
}

export function formatCompactTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return String(value)
}

export function buildTokenTrendSpec(
  series: TokenTrendSeries[],
  label: (key: string) => string
): Record<string, unknown> {
  const seriesLabels = {
    input: label('Input'),
    cacheRead: label('Cache Read'),
    cacheWrite: label('Cache Write'),
    output: label('Output'),
    cacheHitRate: label('Cache hit rate'),
  }

  // VChart has no ECharts-style seriesKey/seriesName: legend labels come from
  // series.name, and the default tooltip keys fall back to raw datum values
  // (numbers, or the time when the value is 0). Each series therefore carries
  // a display name plus an explicit tooltip content override.
  const seriesTooltip = (key: string, field: string, percent = false) => {
    const value = (datum: Record<string, unknown>) => {
      const raw = Number(datum?.[field] ?? 0)
      return percent ? `${raw.toFixed(1)}%` : formatCompactTokens(raw)
    }
    const content = [{ key, value }]
    return { tooltip: { mark: { content }, dimension: { content } } }
  }

  // One data view per series (full rows, cloned): VChart 2.x mutates stacked
  // rows in place, so stacked series sharing a single data view corrupt each
  // other's cumulative offsets and the token axis max collapses.
  const seriesData = ['input', 'cacheRead', 'cacheWrite', 'output', 'cacheHitRate'].map(
    (field) => ({
      id: `trend-${field}`,
      values: series.map((row) => ({ ...row })),
    })
  )
  const dataId = (field: string) => `trend-${field}`

  return {
    type: 'common',
    theme: 'light',
    background: 'transparent',
    data: seriesData,
    series: [
      {
        type: 'area',
        xField: 'time',
        yField: 'input',
        id: 'input',
        name: seriesLabels.input,
        dataId: dataId('input'),
        stack: true,
        line: { style: { stroke: '#3b82f6', lineWidth: 1.5 } },
        area: { style: { fill: '#3b82f6', fillOpacity: 0.2 } },
        point: { visible: false },
        ...seriesTooltip(seriesLabels.input, 'input'),
      },
      {
        type: 'area',
        xField: 'time',
        yField: 'cacheRead',
        id: 'cacheRead',
        name: seriesLabels.cacheRead,
        dataId: dataId('cacheRead'),
        stack: true,
        line: { style: { stroke: '#06b6d4', lineWidth: 1.5 } },
        area: { style: { fill: '#06b6d4', fillOpacity: 0.18 } },
        point: { visible: false },
        ...seriesTooltip(seriesLabels.cacheRead, 'cacheRead'),
      },
      {
        type: 'area',
        xField: 'time',
        yField: 'cacheWrite',
        id: 'cacheWrite',
        name: seriesLabels.cacheWrite,
        dataId: dataId('cacheWrite'),
        stack: true,
        line: { style: { stroke: '#f59e0b', lineWidth: 1.5 } },
        area: { style: { fill: '#f59e0b', fillOpacity: 0.18 } },
        point: { visible: false },
        ...seriesTooltip(seriesLabels.cacheWrite, 'cacheWrite'),
      },
      {
        type: 'area',
        xField: 'time',
        yField: 'output',
        id: 'output',
        name: seriesLabels.output,
        dataId: dataId('output'),
        stack: false,
        line: { style: { stroke: '#10b981', lineWidth: 1.5 } },
        area: { style: { fill: '#10b981', fillOpacity: 0.18 } },
        point: { visible: false },
        ...seriesTooltip(seriesLabels.output, 'output'),
      },
      {
        type: 'line',
        xField: 'time',
        yField: 'cacheHitRate',
        id: 'cacheHitRate',
        name: seriesLabels.cacheHitRate,
        dataId: dataId('cacheHitRate'),
        line: {
          style: { stroke: '#8b5cf6', lineWidth: 2, lineDash: [5, 5] },
        },
        point: { style: { fill: '#8b5cf6', size: 6 } },
        label: { visible: false },
        ...seriesTooltip(seriesLabels.cacheHitRate, 'cacheHitRate', true),
      },
    ],
    axes: [
      { orient: 'bottom', type: 'band', trimPadding: true },
      {
        orient: 'left',
        id: 'yTokens',
        type: 'linear',
        // VChart binds series to axes via the axis's seriesId (series-side
        // axisId is ignored); without the split, every series — including the
        // hit-rate line — renders against whichever linear axis comes first.
        seriesId: ['input', 'cacheRead', 'cacheWrite', 'output'],
        label: {
          formatMethod: (value: number) => formatCompactTokens(Number(value)),
        },
      },
      {
        orient: 'right',
        id: 'yPercent',
        seriesId: ['cacheHitRate'],
        type: 'linear',
        min: 0,
        max: 100,
        label: { formatMethod: (value: number) => `${value}%` },
        grid: { visible: false },
      },
    ],
    legends: [{ visible: true, orient: 'top', position: 'start' }],
  }
}
