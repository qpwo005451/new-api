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
  applyPresetToModel,
  findActivePresetName,
  findActiveScopedPresetName,
  type ModelWeightOverrideEntry,
  normalizeRatioWeights,
  parseModelWeightPresets,
  presetsForModel,
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

describe('model weight preset scope (T01)', () => {
  it('round-trips a scoped preset with the scope trimmed', () => {
    const raw = JSON.stringify([
      {
        name: '仅 flash',
        model: '  DeepSeek-V4.1-Flash  ',
        weights: [{ channel_id: 9, model: 'deepseek-v4.1-flash', weight: 100 }],
      },
    ])
    const parsed = parseModelWeightPresets(raw)
    expect(parsed[0].model).toBe('DeepSeek-V4.1-Flash')
    expect(JSON.parse(serializeModelWeightPresets(parsed))).toEqual([
      {
        name: '仅 flash',
        model: 'DeepSeek-V4.1-Flash',
        weights: [{ channel_id: 9, model: 'deepseek-v4.1-flash', weight: 100 }],
      },
    ])
  })

  it('omits the model key for a global preset so old bytes are unchanged', () => {
    const raw = JSON.stringify([
      {
        name: '均衡',
        weights: [{ channel_id: 9, model: 'glm-5.3-flash', weight: 50 }],
      },
    ])
    const parsed = parseModelWeightPresets(raw)
    expect(parsed).toEqual([
      {
        name: '均衡',
        weights: [{ channel_id: 9, model: 'glm-5.3-flash', weight: 50 }],
      },
    ])
    expect(serializeModelWeightPresets(parsed)).toBe(raw)
  })

  it('treats a blank or non-string scope as a global preset', () => {
    const parsed = parseModelWeightPresets(
      JSON.stringify([
        { name: 'blank', model: '   ', weights: [] },
        { name: 'number', model: 42, weights: [] },
      ])
    )
    expect(parsed).toEqual([
      { name: 'blank', weights: [] },
      { name: 'number', weights: [] },
    ])
    expect(serializeModelWeightPresets(parsed)).toBe(
      JSON.stringify([
        { name: 'blank', weights: [] },
        { name: 'number', weights: [] },
      ])
    )
  })
})

describe('presetsForModel', () => {
  const presets = parseModelWeightPresets(
    JSON.stringify([
      { name: 'global', weights: [] },
      { name: 'flash', model: 'DeepSeek-V4.1-Flash', weights: [] },
      { name: 'glm', model: 'glm-4.6', weights: [] },
    ])
  )

  it('returns only the presets scoped to the model, case-insensitively', () => {
    expect(
      presetsForModel(presets, '  deepseek-v4.1-FLASH  ').map(
        (preset) => preset.name
      )
    ).toEqual(['flash'])
  })

  it('never returns global presets', () => {
    expect(presetsForModel(presets, '')).toEqual([])
  })
})

describe('applyPresetToModel', () => {
  const current = JSON.stringify([
    { channel_id: 1, model: 'glm-4.6', weight: 100 },
    { channel_id: 9, model: 'deepseek-v4.1-flash', priority: 501, weight: 700 },
    {
      channel_id: 36,
      model: 'deepseek-v4.1-flash',
      priority: 501,
      weight: 300,
    },
    { channel_id: 2, model: 'kimi-k2', weight: 100 },
  ])

  it('replaces only the target model rows at their first position, keeping other models in order (T08)', () => {
    const [preset] = parseModelWeightPresets(
      JSON.stringify([
        {
          name: 'flash 均衡',
          model: 'DeepSeek-V4.1-Flash',
          weights: [
            {
              channel_id: 9,
              model: 'deepseek-v4.1-flash',
              priority: 501,
              weight: 600,
            },
            {
              channel_id: 46,
              model: 'deepseek-v4.1-flash',
              priority: 501,
              weight: 400,
            },
          ],
        },
      ])
    )
    const result: ModelWeightOverrideEntry[] = JSON.parse(
      applyPresetToModel(current, preset)
    )
    expect(result).toEqual([
      { channel_id: 1, model: 'glm-4.6', weight: 100 },
      {
        channel_id: 9,
        model: 'deepseek-v4.1-flash',
        priority: 501,
        weight: 600,
      },
      {
        channel_id: 46,
        model: 'deepseek-v4.1-flash',
        priority: 501,
        weight: 400,
      },
      { channel_id: 2, model: 'kimi-k2', weight: 100 },
    ])
  })

  it('appends the preset rows when the target model has no rows (T09)', () => {
    const [preset] = parseModelWeightPresets(
      JSON.stringify([
        {
          name: 'kimi-k3 均衡',
          model: 'kimi-k3',
          weights: [{ channel_id: 7, model: 'kimi-k3', weight: 100 }],
        },
      ])
    )
    const result: ModelWeightOverrideEntry[] = JSON.parse(
      applyPresetToModel(current, preset)
    )
    expect(result).toEqual([
      ...JSON.parse(current),
      { channel_id: 7, model: 'kimi-k3', weight: 100 },
    ])
  })

  it('replaces the whole table for a global preset (T10)', () => {
    const [preset] = parseModelWeightPresets(
      JSON.stringify([
        {
          name: '全局',
          weights: [{ channel_id: 3, model: 'glm-4.6', weight: 100 }],
        },
      ])
    )
    expect(JSON.parse(applyPresetToModel(current, preset))).toEqual([
      { channel_id: 3, model: 'glm-4.6', weight: 100 },
    ])
  })

  it('drops duplicate (channel_id, model) pairs while merging', () => {
    const [preset] = parseModelWeightPresets(
      JSON.stringify([
        {
          name: '重复',
          model: 'deepseek-v4.1-flash',
          weights: [
            { channel_id: 9, model: 'deepseek-v4.1-flash', weight: 100 },
            { channel_id: 9, model: 'DeepSeek-V4.1-Flash', weight: 200 },
          ],
        },
      ])
    )
    const result: ModelWeightOverrideEntry[] = JSON.parse(
      applyPresetToModel(current, preset)
    )
    const pairs = result.map(
      (entry) => `${entry.channel_id}|${entry.model.trim().toLowerCase()}`
    )
    expect(new Set(pairs).size).toBe(pairs.length)
    expect(result).toContainEqual({
      channel_id: 9,
      model: 'deepseek-v4.1-flash',
      weight: 100,
    })
  })
})

describe('findActiveScopedPresetName', () => {
  const presets = parseModelWeightPresets(
    JSON.stringify([
      {
        name: '全局均衡',
        weights: [{ channel_id: 1, model: 'glm-4.6', weight: 100 }],
      },
      {
        name: 'flash 均衡',
        model: 'deepseek-v4.1-flash',
        weights: [
          {
            channel_id: 9,
            model: 'deepseek-v4.1-flash',
            priority: 501,
            weight: 700,
          },
          {
            channel_id: 36,
            model: 'deepseek-v4.1-flash',
            priority: 501,
            weight: 300,
          },
        ],
      },
    ])
  )

  it('matches the target model rows while other models differ (T11)', () => {
    const live = JSON.stringify([
      { channel_id: 2, model: 'kimi-k2', weight: 100 },
      {
        channel_id: 9,
        model: 'deepseek-v4.1-flash',
        priority: 501,
        weight: 700,
      },
      {
        channel_id: 36,
        model: 'deepseek-v4.1-flash',
        priority: 501,
        weight: 300,
      },
    ])
    expect(
      findActiveScopedPresetName(presets, 'deepseek-v4.1-flash', live)
    ).toBe('flash 均衡')
  })

  it('matches regardless of row order and model case (T12)', () => {
    const live = JSON.stringify([
      {
        channel_id: 36,
        model: 'DeepSeek-V4.1-Flash',
        priority: 501,
        weight: 300,
      },
      {
        channel_id: 9,
        model: 'DEEPSEEK-V4.1-FLASH',
        priority: 501,
        weight: 700,
      },
    ])
    expect(
      findActiveScopedPresetName(presets, '  deepseek-v4.1-FLASH  ', live)
    ).toBe('flash 均衡')
  })

  it('returns null when the target model rows match no scoped preset', () => {
    const live = JSON.stringify([
      {
        channel_id: 9,
        model: 'deepseek-v4.1-flash',
        priority: 501,
        weight: 100,
      },
    ])
    expect(
      findActiveScopedPresetName(presets, 'deepseek-v4.1-flash', live)
    ).toBeNull()
  })

  it('never matches a global preset for a scoped lookup', () => {
    const live = JSON.stringify([
      { channel_id: 1, model: 'glm-4.6', weight: 100 },
    ])
    expect(findActiveScopedPresetName(presets, '', live)).toBeNull()
  })
})

describe('normalizeRatioWeights', () => {
  it('keeps positive integer percentages as weights (T13)', () => {
    expect(normalizeRatioWeights([70, 30])).toEqual([70, 30])
  })

  it('never rounds a positive share into exclusion and keeps zero at zero (T14)', () => {
    expect(normalizeRatioWeights([0.4, 99.6])).toEqual([1, 100])
    expect(normalizeRatioWeights([0, 100])).toEqual([0, 100])
  })

  it('turns non-positive or non-finite values into zero and clamps large values (T15)', () => {
    expect(
      normalizeRatioWeights([
        -5,
        Number.NaN,
        Number.POSITIVE_INFINITY,
        2_000_000,
      ])
    ).toEqual([0, 0, 0, 1_000_000])
  })
})
