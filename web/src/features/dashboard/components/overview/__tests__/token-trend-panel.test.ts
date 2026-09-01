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
import { describe, expect, test } from 'vitest'

import {
  buildTokenTrendSeries,
  formatCompactTokens,
} from '@/features/dashboard/lib/token-trend'
import type { TokenTrendPoint } from '@/features/dashboard/types'

function makePoint(overrides: Partial<TokenTrendPoint>): TokenTrendPoint {
  return {
    created_at: 1700000000,
    requests: 1,
    prompt_tokens: 0,
    completion_tokens: 0,
    cache_read: 0,
    cache_write: 0,
    cache_prompt_tokens: 0,
    ...overrides,
  }
}

describe('buildTokenTrendSeries', () => {
  test('subtracts cache read and write from prompt tokens for plain input', () => {
    const series = buildTokenTrendSeries([
      makePoint({
        prompt_tokens: 1000,
        completion_tokens: 200,
        cache_read: 400,
        cache_write: 100,
        cache_prompt_tokens: 1000,
      }),
    ])

    expect(series).toHaveLength(1)
    expect(series[0].input).toBe(500)
    expect(series[0].output).toBe(200)
    expect(series[0].cacheRead).toBe(400)
    expect(series[0].cacheWrite).toBe(100)
  })

  test('computes cache hit rate as cache read over cache prompt tokens', () => {
    const series = buildTokenTrendSeries([
      makePoint({
        prompt_tokens: 1000,
        cache_read: 400,
        cache_write: 100,
        cache_prompt_tokens: 1000,
      }),
    ])

    expect(series[0].cacheHitRate).toBe(40)
  })

  test('clamps input to zero when cache tokens exceed prompt tokens', () => {
    const series = buildTokenTrendSeries([
      makePoint({
        prompt_tokens: 100,
        cache_read: 150,
        cache_write: 50,
        cache_prompt_tokens: 100,
      }),
    ])

    expect(series[0].input).toBe(0)
  })

  test('returns zero hit rate for a bucket without tokens', () => {
    const series = buildTokenTrendSeries([makePoint({})])

    expect(series[0].cacheHitRate).toBe(0)
    expect(series[0].input).toBe(0)
  })

  test('maps each hourly bucket to one row preserving order', () => {
    const series = buildTokenTrendSeries([
      makePoint({
        created_at: 3600 * 5,
        prompt_tokens: 10,
        cache_prompt_tokens: 10,
      }),
      makePoint({
        created_at: 3600 * 6,
        prompt_tokens: 20,
        cache_prompt_tokens: 20,
      }),
    ])

    // Labels are local-time "MM-DD HH:00" strings; consecutive hourly buckets
    // stay ordered one hour apart.
    expect(series.map((row) => row.time)).toEqual([
      expect.stringMatching(/^\d{2}-\d{2} \d{2}:00$/),
      expect.stringMatching(/^\d{2}-\d{2} \d{2}:00$/),
    ])
    expect(series[0].input).toBe(10)
    expect(series[1].input).toBe(20)
  })

  test('feeds the panel an 80% total hit rate from 80 read over 100 cache-prompt tokens', () => {
    // Two buckets totaling 80 cache-read over 100 cache-prompt tokens: the
    // panel badge must show 80%, not the 4% implied by the full 2000
    // prompt tokens (Ollama traffic without cache reporting).
    const series = buildTokenTrendSeries([
      makePoint({
        prompt_tokens: 500,
        cache_prompt_tokens: 50,
        cache_read: 40,
        cache_write: 10,
      }),
      makePoint({
        prompt_tokens: 1500,
        cache_prompt_tokens: 50,
        cache_read: 40,
      }),
    ])

    expect(series.map((row) => row.cacheHitRate)).toEqual([80, 80])
    expect(series.map((row) => row.cachePrompt)).toEqual([50, 50])
  })
})

describe('formatCompactTokens', () => {
  test.each([
    [0, '0'],
    [999, '999'],
    [1500, '1.5K'],
    [53_670_000, '53.67M'],
    [2_300_000_000, '2.30B'],
  ])('formats %d as %s', (input, expected) => {
    expect(formatCompactTokens(input)).toBe(expected)
  })
})
