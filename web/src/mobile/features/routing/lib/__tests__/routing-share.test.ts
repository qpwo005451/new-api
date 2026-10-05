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
  summarizeRoutingShare,
  topSwitches,
} from '@/mobile/features/routing/lib/routing-share'
import type { RoutingModelChannelStat, RoutingSwitchStat } from '@/mobile/types'

const rows: RoutingModelChannelStat[] = [
  {
    model_name: 'gpt-5',
    channel_id: 1,
    channel_name: 'a',
    requests: 100,
    errors: 2,
    avg_use_time: 1.5,
    priority: 0,
    weight: 10,
  },
  {
    model_name: 'gpt-5',
    channel_id: 2,
    channel_name: 'b',
    requests: 300,
    errors: 0,
    avg_use_time: 1.1,
    priority: 0,
    weight: 30,
  },
  {
    model_name: 'claude-5',
    channel_id: 3,
    channel_name: 'c',
    requests: 0,
    errors: 0,
    avg_use_time: 0,
  },
]

describe('summarizeRoutingShare', () => {
  it('compares observed share against the configured weight share', () => {
    const [gpt5] = summarizeRoutingShare(rows)

    expect(gpt5.modelName).toBe('gpt-5')
    expect(gpt5.requests).toBe(400)
    expect(gpt5.errors).toBe(2)
    expect(gpt5.configuredShare).toBeCloseTo(1)
    expect(gpt5.channels[0]).toMatchObject({ channelName: 'b', requests: 300 })
    expect(gpt5.channels[0].configuredShare).toBeCloseTo(0.75)
    expect(gpt5.channels[1].configuredShare).toBeCloseTo(0.25)
  })

  it('marks the configured share as unknown when no channel reports a weight', () => {
    const [claude] = summarizeRoutingShare([rows[2]])

    expect(claude.configuredShare).toBeNull()
    expect(claude.channels[0].configuredShare).toBeNull()
  })

  it('returns an empty list for no rows', () => {
    expect(summarizeRoutingShare([])).toEqual([])
  })
})

describe('topSwitches', () => {
  it('sorts by count and truncates', () => {
    const switches: RoutingSwitchStat[] = [
      { from: 1, to: 2, count: 3 },
      { from: 2, to: 3, count: 9 },
    ]

    expect(topSwitches(switches, 1)).toEqual([{ from: 2, to: 3, count: 9 }])
  })
})
