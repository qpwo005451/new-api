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

/**
 * Named, ready-to-apply per-model routing presets.
 *
 * A preset stores the same override entries as `model_weight_setting.weights`,
 * so applying one is a single option write and the active preset can be derived
 * by comparing the live weights with each preset instead of persisting extra
 * state that could drift.
 */
export type ModelWeightOverrideEntry = {
  channel_id: number
  model: string
  weight?: number
  priority?: number
}

export type ModelWeightPreset = {
  name: string
  weights: ModelWeightOverrideEntry[]
}

export const MODEL_WEIGHTS_OPTION_KEY = 'model_weight_setting.weights'
export const MODEL_WEIGHT_PRESETS_OPTION_KEY = 'model_weight_setting.presets'

const PRESET_NAME_MAX_LENGTH = 64

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function toOverrideEntry(value: unknown): ModelWeightOverrideEntry | null {
  if (!isRecord(value)) return null
  const channelID = Number(value.channel_id)
  const model = typeof value.model === 'string' ? value.model.trim() : ''
  if (!Number.isFinite(channelID) || channelID <= 0 || model === '') return null

  const entry: ModelWeightOverrideEntry = { channel_id: channelID, model }
  if (
    value.weight !== undefined &&
    value.weight !== null &&
    value.weight !== ''
  ) {
    entry.weight = Number(value.weight)
  }
  if (
    value.priority !== undefined &&
    value.priority !== null &&
    value.priority !== ''
  ) {
    entry.priority = Number(value.priority)
  }
  if (entry.weight === undefined && entry.priority === undefined) return null
  return entry
}

function toOverrideList(value: unknown): ModelWeightOverrideEntry[] {
  if (!Array.isArray(value)) return []
  return value
    .map(toOverrideEntry)
    .filter((entry): entry is ModelWeightOverrideEntry => entry !== null)
}

function canonicalEntry(entry: ModelWeightOverrideEntry) {
  const item: Record<string, number | string> = {
    channel_id: entry.channel_id,
    model: entry.model.trim().toLowerCase(),
  }
  if (entry.weight !== undefined) item.weight = entry.weight
  if (entry.priority !== undefined) item.priority = entry.priority
  return item
}

/** Order- and case-insensitive key so two equivalent configs compare equal. */
function canonicalKey(entries: ModelWeightOverrideEntry[]): string {
  return JSON.stringify(
    entries.map(canonicalEntry).sort((left, right) => {
      const channelDiff = Number(left.channel_id) - Number(right.channel_id)
      if (channelDiff !== 0) return channelDiff
      return String(left.model).localeCompare(String(right.model))
    })
  )
}

/** Parse the raw `model_weight_setting.presets` option, dropping malformed items. */
export function parseModelWeightPresets(raw: string): ModelWeightPreset[] {
  try {
    const parsed: unknown = JSON.parse(raw || '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.flatMap((item) => {
      if (!isRecord(item) || typeof item.name !== 'string') return []
      const name = item.name.trim()
      if (name === '' || name.length > PRESET_NAME_MAX_LENGTH) return []
      return [{ name, weights: toOverrideList(item.weights) }]
    })
  } catch {
    return []
  }
}

/** Parse the raw `model_weight_setting.weights` option into override entries. */
export function parseModelWeightEntries(
  raw: string
): ModelWeightOverrideEntry[] {
  try {
    return toOverrideList(JSON.parse(raw || '[]'))
  } catch {
    return []
  }
}

function serializeEntries(entries: ModelWeightOverrideEntry[]) {
  return entries.map((entry) => {
    const item: Record<string, number | string> = {
      channel_id: entry.channel_id,
      model: entry.model.trim(),
    }
    if (entry.weight !== undefined) item.weight = entry.weight
    if (entry.priority !== undefined) item.priority = entry.priority
    return item
  })
}

export function serializeModelWeightPresets(
  presets: ModelWeightPreset[]
): string {
  return JSON.stringify(
    presets.map((preset) => ({
      name: preset.name.trim(),
      weights: serializeEntries(preset.weights),
    }))
  )
}

/** Serialize a preset's weights for the `model_weight_setting.weights` option. */
export function serializePresetWeights(preset: ModelWeightPreset): string {
  return JSON.stringify(serializeEntries(preset.weights))
}

/** Name of the preset that matches the live weights, or null for a custom config. */
export function findActivePresetName(
  presets: ModelWeightPreset[],
  weightsRaw: string
): string | null {
  const target = canonicalKey(parseModelWeightEntries(weightsRaw))
  for (const preset of presets) {
    if (canonicalKey(preset.weights) === target) return preset.name
  }
  return null
}
