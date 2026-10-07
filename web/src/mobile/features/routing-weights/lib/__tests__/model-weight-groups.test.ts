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
  applyNormalizedWeights,
  buildModelWeightGroups,
  type ChannelWeightLookup,
} from '@/mobile/features/routing-weights/lib/model-weight-groups'

const channels: ChannelWeightLookup = new Map([
  [9, { name: 'primary-openai', priority: 501, weight: 1000 }],
  [36, { name: 'ollama', priority: 501, weight: 1000 }],
  [7, { name: 'edge', priority: 800, weight: 1000 }],
])

describe('buildModelWeightGroups', () => {
  it('groups by first appearance and sorts each group by priority then channel id', () => {
    const groups = buildModelWeightGroups(
      [
        { channel_id: 9, model: 'gpt-5', weight: 700 },
        { channel_id: 36, model: 'gpt-5', weight: 300 },
        { channel_id: 7, model: 'gpt-5', weight: 1 },
        { channel_id: 9, model: 'glm-4.6', weight: 500 },
      ],
      channels
    )

    expect(groups.map((group) => group.model)).toEqual(['gpt-5', 'glm-4.6'])
    expect(groups[0].rows.map((row) => row.entry.channel_id)).toEqual([
      7, 9, 36,
    ])
  })

  it('computes shares inside each priority tier from the effective weights', () => {
    const groups = buildModelWeightGroups(
      [
        { channel_id: 9, model: 'gpt-5', weight: 700 },
        { channel_id: 36, model: 'gpt-5', weight: 300 },
        { channel_id: 7, model: 'gpt-5', weight: 1 },
      ],
      channels
    )

    const rows = groups[0].rows
    // The 501 tier is 700:300 regardless of the 800 tier's single row.
    expect(rows[0].entry.channel_id).toBe(7)
    expect(rows[0].sharePercent).toBe(100)
    expect(rows[1].entry.channel_id).toBe(9)
    expect(rows[1].sharePercent).toBe(70)
    expect(rows[2].entry.channel_id).toBe(36)
    expect(rows[2].sharePercent).toBe(30)
    expect(groups[0].hasUnavailableShare).toBe(false)
  })

  it('falls back to the channel weight when the row has no override', () => {
    const groups = buildModelWeightGroups(
      [
        { channel_id: 9, model: 'gpt-5', priority: 501 },
        { channel_id: 36, model: 'gpt-5', priority: 501 },
      ],
      channels
    )

    expect(groups[0].rows[0].weight).toBe(1000)
    expect(groups[0].rows[0].sharePercent).toBe(50)
    expect(groups[0].rows[1].sharePercent).toBe(50)
  })

  it('shows no share when a channel weight is unknown', () => {
    const groups = buildModelWeightGroups(
      [
        { channel_id: 9, model: 'gpt-5', priority: 501 },
        { channel_id: 999, model: 'gpt-5', priority: 501 },
      ],
      channels
    )

    // The known row keeps its effective weight, but the tier cannot be
    // trusted while its peer is unknown, so neither share is printed.
    expect(groups[0].rows[0].weight).toBe(1000)
    expect(groups[0].rows[1].weight).toBeNull()
    expect(groups[0].rows[0].sharePercent).toBeNull()
    expect(groups[0].rows[1].sharePercent).toBeNull()
    expect(groups[0].hasUnavailableShare).toBe(true)
  })

  it('shows no share when the tier weight sum is zero', () => {
    const groups = buildModelWeightGroups(
      [
        { channel_id: 9, model: 'gpt-5', weight: 0 },
        { channel_id: 36, model: 'gpt-5', weight: 0 },
      ],
      channels
    )

    expect(groups[0].rows[0].sharePercent).toBeNull()
    expect(groups[0].hasUnavailableShare).toBe(true)
  })
})

describe('applyNormalizedWeights', () => {
  it('replaces only the target model weights and keeps priority and order', () => {
    const entries = [
      { channel_id: 9, model: 'gpt-5', weight: 700, priority: 10 },
      { channel_id: 36, model: 'gpt-5', weight: 300, priority: 10 },
      { channel_id: 9, model: 'glm-4.6', weight: 500 },
    ]

    const next = applyNormalizedWeights(
      entries,
      'GPT-5',
      new Map([
        [9, 1],
        [36, 100],
      ])
    )

    expect(next).toEqual([
      { channel_id: 9, model: 'gpt-5', weight: 1, priority: 10 },
      { channel_id: 36, model: 'gpt-5', weight: 100, priority: 10 },
      { channel_id: 9, model: 'glm-4.6', weight: 500 },
    ])
  })
})
