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
  buildDistributionSpec,
  buildLiveAllocationSpec,
  buildReasonsSpec,
  buildTrendSpec,
} from '../charts'
import type { ChannelAffinityBinding } from '../types'

const label = (key: string) => `[${key}]`

function values(
  spec: Record<string, unknown> | null
): Record<string, unknown>[] {
  const data = spec?.data as { values: Record<string, unknown>[] }[]
  return data[0].values
}

describe('buildDistributionSpec', () => {
  it('renders nothing without data', () => {
    expect(buildDistributionSpec([], label)).toBeNull()
  })

  it('reports the observed share inside the model beside the configured share', () => {
    const spec = buildDistributionSpec(
      [
        {
          model_name: 'deepseek-v4.1-flash',
          channel_id: 9,
          channel_name: 'input-0.1X',
          requests: 8,
          errors: 0,
          avg_use_time: 1,
          priority: 500,
          weight: 80,
        },
        {
          model_name: 'deepseek-v4.1-flash',
          channel_id: 47,
          channel_name: 'commandcode',
          requests: 2,
          errors: 0,
          avg_use_time: 1,
          priority: 500,
          weight: 20,
        },
      ],
      label
    )

    expect(values(spec)).toEqual([
      {
        model: 'deepseek-v4.1-flash',
        channel: 'input-0.1X',
        requests: 8,
        share: 80,
        configured: '80%',
      },
      {
        model: 'deepseek-v4.1-flash',
        channel: 'commandcode',
        requests: 2,
        share: 20,
        configured: '20%',
      },
    ])
  })

  it('labels a channel without a name by its id', () => {
    const spec = buildDistributionSpec(
      [
        {
          model_name: 'm',
          channel_id: 9,
          channel_name: '',
          requests: 1,
          errors: 0,
          avg_use_time: 1,
        },
      ],
      label
    )

    expect(values(spec)[0].channel).toBe('#9')
    expect(values(spec)[0].configured).toBe('-')
  })
})

describe('buildTrendSpec', () => {
  it('renders nothing without data', () => {
    expect(buildTrendSpec([], label)).toBeNull()
  })

  it('draws one labelled line per metric', () => {
    const spec = buildTrendSpec(
      [
        { time: '10-02 06:00', requests: 4, switched: 1 },
        { time: '10-02 07:00', requests: 6, switched: 2 },
      ],
      label
    )

    const series = spec?.series as { name: string; yField: string }[]
    expect(series.map((item) => item.name)).toEqual([
      '[Requests]',
      '[Switched requests]',
    ])
    expect(series.map((item) => item.yField)).toEqual(['requests', 'switched'])
  })
})

describe('buildReasonsSpec', () => {
  it('renders nothing without data', () => {
    expect(buildReasonsSpec([], label)).toBeNull()
  })

  it('labels each reason through the translation function', () => {
    const spec = buildReasonsSpec(
      [
        { reason: 'channel_error', count: 3 },
        { reason: 'retry_status_matched', count: 1 },
      ],
      label
    )

    expect(values(spec)).toEqual([
      { reason: '[channel_error]', count: 3 },
      { reason: '[retry_status_matched]', count: 1 },
    ])
  })
})

describe('buildLiveAllocationSpec', () => {
  it('renders nothing when no session is pinned', () => {
    expect(buildLiveAllocationSpec([], label)).toBeNull()
  })

  it('measures the session share inside each model', () => {
    const entries: ChannelAffinityBinding[] = [
      {
        rule_name: 'r',
        model_name: 'deepseek-v4.1-flash',
        using_group: 'default',
        key_hint: 'a',
        key_fingerprint: 'a',
        channel_id: 9,
        channel_name: 'input-0.1X',
      },
      {
        rule_name: 'r',
        model_name: 'deepseek-v4.1-flash',
        using_group: 'default',
        key_hint: 'b',
        key_fingerprint: 'b',
        channel_id: 47,
        channel_name: 'commandcode',
      },
      {
        rule_name: 'r',
        model_name: 'deepseek-v4.1-flash',
        using_group: 'default',
        key_hint: 'c',
        key_fingerprint: 'c',
        channel_id: 47,
        channel_name: 'commandcode',
      },
    ]

    // Busiest channel first, which is the order the allocation table uses too.
    expect(values(buildLiveAllocationSpec(entries, label))).toEqual([
      {
        model: 'deepseek-v4.1-flash',
        channel: 'commandcode',
        sessions: 2,
        share: 66.67,
      },
      {
        model: 'deepseek-v4.1-flash',
        channel: 'input-0.1X',
        sessions: 1,
        share: 33.33,
      },
    ])
  })
})
