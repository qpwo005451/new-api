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
  findActivePresetName,
  parseModelWeightPresets,
  serializeModelWeightPresets,
  serializePresetWeights,
} from '../model-weight-presets'

describe('parseModelWeightPresets', () => {
  it('parses named presets and keeps their weights', () => {
    const presets = parseModelWeightPresets(
      JSON.stringify([
        {
          name: '全部 ollama',
          weights: [
            {
              channel_id: 46,
              model: 'deepseek-v4.1-flash',
              priority: 501,
              weight: 100,
            },
          ],
        },
      ])
    )
    expect(presets).toEqual([
      {
        name: '全部 ollama',
        weights: [
          {
            channel_id: 46,
            model: 'deepseek-v4.1-flash',
            priority: 501,
            weight: 100,
          },
        ],
      },
    ])
  })

  it('returns an empty list for malformed input', () => {
    expect(parseModelWeightPresets('')).toEqual([])
    expect(parseModelWeightPresets('not json')).toEqual([])
    expect(parseModelWeightPresets('{"name":"x"}')).toEqual([])
  })

  it('drops entries without a name and trims the name', () => {
    const presets = parseModelWeightPresets(
      JSON.stringify([{ weights: [] }, { name: '  默认  ', weights: [] }])
    )
    expect(presets).toEqual([{ name: '默认', weights: [] }])
  })

  it('drops weight entries that are not usable overrides', () => {
    const presets = parseModelWeightPresets(
      JSON.stringify([
        {
          name: 'a',
          weights: [
            { channel_id: 0, model: 'm', weight: 1 },
            { channel_id: 9, model: '', weight: 1 },
            { channel_id: 9, model: 'm' },
            { channel_id: 9, model: 'm', weight: 5 },
          ],
        },
      ])
    )
    expect(presets[0].weights).toEqual([
      { channel_id: 9, model: 'm', weight: 5 },
    ])
  })
})

describe('serializeModelWeightPresets', () => {
  it('round-trips a preset list', () => {
    const raw = JSON.stringify([
      {
        name: '均衡',
        weights: [
          { channel_id: 9, model: 'glm-5.3-flash', priority: 500, weight: 50 },
        ],
      },
    ])
    expect(
      JSON.parse(serializeModelWeightPresets(parseModelWeightPresets(raw)))
    ).toEqual(JSON.parse(raw))
  })
})

describe('findActivePresetName', () => {
  const presets = parseModelWeightPresets(
    JSON.stringify([
      {
        name: '默认',
        weights: [
          {
            channel_id: 9,
            model: 'deepseek-v4.1-flash',
            priority: 500,
            weight: 50,
          },
          {
            channel_id: 46,
            model: 'deepseek-v4.1-flash',
            priority: 500,
            weight: 10,
          },
        ],
      },
    ])
  )

  it('matches regardless of entry order, model case and numeric formatting', () => {
    const live = JSON.stringify([
      {
        channel_id: 46,
        model: 'DeepSeek-V4.1-Flash',
        weight: 10,
        priority: 500,
      },
      {
        channel_id: 9,
        model: 'deepseek-v4.1-flash',
        priority: 500,
        weight: 50,
      },
    ])
    expect(findActivePresetName(presets, live)).toBe('默认')
  })

  it('returns null when the live weights are a custom configuration', () => {
    const live = JSON.stringify([
      {
        channel_id: 9,
        model: 'deepseek-v4.1-flash',
        priority: 501,
        weight: 50,
      },
    ])
    expect(findActivePresetName(presets, live)).toBeNull()
  })

  it('treats an empty preset as matching an empty configuration', () => {
    const empty = parseModelWeightPresets(
      JSON.stringify([{ name: '原生', weights: [] }])
    )
    expect(findActivePresetName(empty, '[]')).toBe('原生')
  })
})

describe('serializePresetWeights', () => {
  it('serializes only the weights of the preset', () => {
    const [preset] = parseModelWeightPresets(
      JSON.stringify([
        {
          name: 'x',
          weights: [{ channel_id: 9, model: 'm', weight: 5 }],
        },
      ])
    )
    expect(JSON.parse(serializePresetWeights(preset))).toEqual([
      { channel_id: 9, model: 'm', weight: 5 },
    ])
  })
})
