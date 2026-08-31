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
import { beforeAll, describe, expect, it } from 'vitest'

import {
  buildTokenTrendSeries,
  buildTokenTrendSpec,
} from '../../../lib/token-trend'

const zhLabels: Record<string, string> = {
  Input: '输入',
  'Cache Read': '缓存读取',
  'Cache Write': '缓存写入',
  Output: '输出',
  'Cache hit rate': '缓存命中率',
}

const label = (key: string) => zhLabels[key] ?? key

// Left linear axis max from the rendered chart (VChart internals).
function getLinearTokenAxisMax(rawChart: unknown): number {
  const chartInst = (rawChart as { getChart?: () => unknown }).getChart?.() as
    | { getAllComponents: () => Array<Record<string, unknown>> }
    | undefined
  if (!chartInst) return 0
  for (const c of chartInst.getAllComponents()) {
    if (!String(c.type).startsWith('cartesianAxis-linear')) continue
    const getScale = c.getScale as
      | (() => { domain: () => number[] })
      | undefined
    const scale = getScale?.call(c)
    if (!scale) continue
    const domain = scale.domain()
    if (domain[1] > 1) return domain[1]
  }
  return 0
}

function makeSeries() {
  const points = [
    {
      created_at: 1787497200,
      requests: 10,
      prompt_tokens: 1_200_000,
      completion_tokens: 20_000,
      cache_read: 1_000_000,
      cache_write: 60_000,
    },
    {
      created_at: 1787500800,
      requests: 4,
      prompt_tokens: 800_000,
      completion_tokens: 15_000,
      cache_read: 700_000,
      cache_write: 0,
    },
  ]
  return buildTokenTrendSeries(points)
}

describe('token trend VChart spec', () => {
  const series = makeSeries()
  const spec = buildTokenTrendSpec(series, label) as {
    data: Array<{ id: string; values: Array<Record<string, unknown>> }>
    series: Array<Record<string, unknown>>
    axes: Array<{
      orient: string
      seriesId?: string[]
      min?: number
      max?: number
    }>
  }

  it('gives every series a translated display name (legend source)', () => {
    const names = spec.series.map((s) => s.name)
    expect(names).toEqual([
      '输入',
      '缓存读取',
      '缓存写入',
      '输出',
      '缓存命中率',
    ])
  })

  it('drops the unsupported ECharts-style seriesKey/seriesName props', () => {
    for (const s of spec.series) {
      expect(s.seriesKey).toBeUndefined()
      expect(s.seriesName).toBeUndefined()
    }
  })

  it('gives each series its own data view (VChart stack corrupts shared views)', () => {
    const datumList = spec.data
    const dataIds = datumList.map((d) => d.id)
    expect(new Set(dataIds).size).toBe(datumList.length)
    for (const s of spec.series) {
      const id = String(s.dataId)
      expect(dataIds).toContain(id)
      const view = datumList.find((d) => d.id === id)
      // The tooltip content callbacks read raw datum fields, so every view
      // must carry the full row shape.
      expect(view?.values[0]).toHaveProperty(String(s.yField))
      expect(view?.values[0]).toHaveProperty('time')
    }
  })

  it('binds the hit-rate line to the percent axis via axis.seriesId', () => {
    // VChart ignores series-side axisId; without axis.seriesId, the first
    // linear axis claims every series and the hit-rate line hugs zero.
    const tokenAxis = spec.axes.find((a) => a.orient === 'left')
    const percentAxis = spec.axes.find((a) => a.orient === 'right')
    expect(tokenAxis?.seriesId).toEqual([
      'input',
      'cacheRead',
      'cacheWrite',
      'output',
    ])
    expect(percentAxis?.seriesId).toEqual(['cacheHitRate'])
    expect(percentAxis?.min).toBe(0)
    expect(percentAxis?.max).toBe(100)
    for (const s of spec.series) {
      expect(s.axisId).toBeUndefined()
    }
  })

  it('overrides mark and dimension tooltip content with label + formatted value', () => {
    for (const s of spec.series) {
      const tooltip = s.tooltip as {
        mark: { content: Array<Record<string, unknown>> }
        dimension: { content: Array<Record<string, unknown>> }
      }
      for (const activeType of ['mark', 'dimension'] as const) {
        const line = tooltip[activeType].content[0]
        const datum = {
          time: '08-24 09:00',
          [String(s.id)]: s.id === 'cacheHitRate' ? 88.9 : 1_234_567,
        }
        expect(line.key).toBe(s.name)
        const value = (line.value as (d: unknown) => string)(datum)
        if (s.id === 'cacheHitRate') {
          expect(value).toBe('88.9%')
        } else {
          expect(value).toBe('1.23M')
        }
      }
    }
  })

  it('renders zero values with the translated label, not a time fallback', () => {
    const cacheWrite = spec.series.find((s) => s.id === 'cacheWrite')
    const tooltip = cacheWrite?.tooltip as
      | { dimension: { content: Array<Record<string, unknown>> } }
      | undefined
    const line = tooltip?.dimension.content[0]
    const datum = { time: '08-25 16:00', cacheWrite: 0 }
    expect(line?.key).toBe('缓存写入')
    expect(line && (line.value as (d: unknown) => string)(datum)).toBe('0')
  })

  describe('SSR render (real VChart, node mode)', () => {
    // Rendered output is written here for manual visual inspection during
    // development; tests assert on VChart's own legend data instead.
    const pngPath = '/tmp/token-trend-ssr.png'
    let legendItems: Array<Record<string, unknown>> = []
    let tokenAxisMax = 0

    beforeAll(async () => {
      const canvas = (await import('canvas')) as unknown as Record<
        string,
        unknown
      >
      const { vglobal } = await import('@visactor/vrender-core')
      vglobal.setEnv('node', canvas)
      const { default: VChart } = await import('@visactor/vchart')
      const chart = new VChart(
        buildTokenTrendSpec(series, label) as never,
        {
          mode: 'node',
          animation: false,
          width: 720,
          height: 320,
        } as never
      )
      chart.renderSync()
      legendItems = chart.getLegendDataByIndex() as never
      tokenAxisMax = getLinearTokenAxisMax(chart)
      const nodeCanvas = chart.getCanvas() as unknown as {
        toBuffer: (format: string) => Buffer
      }
      ;(await import('node:fs')).writeFileSync(
        pngPath,
        nodeCanvas.toBuffer('image/png')
      )
    }, 30_000)

    it('shows translated legend labels', () => {
      const texts = legendItems.map((item) => String(item.key))
      expect(texts).toEqual([
        '输入',
        '缓存读取',
        '缓存写入',
        '输出',
        '缓存命中率',
      ])
    })

    it('sizes the token axis to the full stacked height', () => {
      // Stack top for point 1: input 140K + cacheRead 1M + cacheWrite 60K.
      // A shared-data stack corruption collapses the axis far below it.
      expect(tokenAxisMax).toBeGreaterThanOrEqual(1_200_000)
    })
  })
})
