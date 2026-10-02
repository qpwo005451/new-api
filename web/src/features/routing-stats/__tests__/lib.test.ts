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
import { describe, expect, it } from 'vitest'

import {
  actualShares,
  buildLiveAllocation,
  buildTrendSeries,
  configuredShares,
  routingChannelKey,
  routingStatsTimeRange,
} from '../lib'
import type { ChannelAffinityBinding } from '../types'

function binding(
  modelName: string,
  channelID: number,
  channelName: string,
  keyHint: string
): ChannelAffinityBinding {
  return {
    rule_name: 'deepseek glm session stickiness',
    model_name: modelName,
    using_group: 'default',
    key_hint: keyHint,
    key_fingerprint: keyHint.padEnd(8, '0'),
    channel_id: channelID,
    channel_name: channelName,
  }
}

describe('routingStatsTimeRange', () => {
  it('covers the preset length and buffers the end of the window', () => {
    expect(routingStatsTimeRange('1h', 1_000_000)).toEqual({
      start_timestamp: 1_000_000 - 3600,
      end_timestamp: 1_000_060,
    })
    expect(routingStatsTimeRange('7d', 1_000_000)).toEqual({
      start_timestamp: 1_000_000 - 7 * 24 * 3600,
      end_timestamp: 1_000_060,
    })
  })
})

describe('configuredShares', () => {
  it('splits the top priority tier by weight', () => {
    const shares = configuredShares([
      { model_name: 'm', channel_id: 9, priority: 500, weight: 50 },
      { model_name: 'm', channel_id: 47, priority: 500, weight: 30 },
      { model_name: 'm', channel_id: 46, priority: 500, weight: 20 },
    ])

    expect(shares.get(routingChannelKey('m', 9))).toBeCloseTo(0.5)
    expect(shares.get(routingChannelKey('m', 47))).toBeCloseTo(0.3)
    expect(shares.get(routingChannelKey('m', 46))).toBeCloseTo(0.2)
  })

  it('gives a lower tier no share because it is only a retry backup', () => {
    const shares = configuredShares([
      { model_name: 'm', channel_id: 9, priority: 500, weight: 100 },
      { model_name: 'm', channel_id: 36, priority: 497, weight: 100 },
    ])

    expect(shares.get(routingChannelKey('m', 9))).toBeCloseTo(1)
    expect(shares.get(routingChannelKey('m', 36))).toBe(0)
  })

  it('falls back to an equal split inside the tier when no weight is set', () => {
    const shares = configuredShares([
      { model_name: 'm', channel_id: 9, priority: 500 },
      { model_name: 'm', channel_id: 47, priority: 500 },
    ])

    expect(shares.get(routingChannelKey('m', 9))).toBeCloseTo(0.5)
    expect(shares.get(routingChannelKey('m', 47))).toBeCloseTo(0.5)
  })

  it('leaves a model out when no channel reports an effective priority', () => {
    const shares = configuredShares([{ model_name: 'm', channel_id: 9 }])

    expect(shares.size).toBe(0)
  })
})

describe('actualShares', () => {
  it('measures the share inside each model', () => {
    const shares = actualShares([
      { model_name: 'a', channel_id: 9, requests: 8 },
      { model_name: 'a', channel_id: 47, requests: 2 },
      { model_name: 'b', channel_id: 9, requests: 5 },
    ])

    expect(shares.get(routingChannelKey('a', 9))).toBeCloseTo(0.8)
    expect(shares.get(routingChannelKey('a', 47))).toBeCloseTo(0.2)
    expect(shares.get(routingChannelKey('b', 9))).toBeCloseTo(1)
  })
})

describe('buildTrendSeries', () => {
  it('labels minute buckets with the time of day', () => {
    const series = buildTrendSeries(
      [{ timestamp: 1_700_000_000, requests: 12, switched: 3 }],
      120
    )

    expect(series).toHaveLength(1)
    expect(series[0].requests).toBe(12)
    expect(series[0].switched).toBe(3)
    expect(series[0].time).toMatch(/^\d{2}-\d{2} \d{2}:\d{2}$/)
  })

  it('labels hour or wider buckets without minutes', () => {
    const series = buildTrendSeries(
      [{ timestamp: 1_700_000_000, requests: 1, switched: 0 }],
      3600
    )

    expect(series[0].time).toMatch(/^\d{2}-\d{2} \d{2}:00$/)
  })
})

describe('buildLiveAllocation', () => {
  it('counts the sessions each model holds per channel', () => {
    const rows = buildLiveAllocation([
      binding('deepseek-v4.1-flash', 9, 'input-0.1X', 'session-a'),
      binding('deepseek-v4.1-flash', 9, 'input-0.1X', 'session-b'),
      binding('deepseek-v4.1-flash', 47, 'commandcode', 'session-c'),
      binding('glm-5.3-flash', 46, 'ollama', 'session-d'),
    ])

    expect(rows).toEqual([
      {
        modelName: 'deepseek-v4.1-flash',
        channelID: 9,
        channelName: 'input-0.1X',
        sessions: 2,
      },
      {
        modelName: 'deepseek-v4.1-flash',
        channelID: 47,
        channelName: 'commandcode',
        sessions: 1,
      },
      {
        modelName: 'glm-5.3-flash',
        channelID: 46,
        channelName: 'ollama',
        sessions: 1,
      },
    ])
  })

  it('returns nothing when no session is pinned', () => {
    expect(buildLiveAllocation([])).toEqual([])
  })
})
