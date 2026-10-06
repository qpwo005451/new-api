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
  /** Empty/absent = global preset (whole table); non-empty = scoped to one model. */
  model?: string
  weights: ModelWeightOverrideEntry[]
}

export const MODEL_WEIGHTS_OPTION_KEY = 'model_weight_setting.weights'
export const MODEL_WEIGHT_PRESETS_OPTION_KEY = 'model_weight_setting.presets'

const PRESET_NAME_MAX_LENGTH = 64

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

/** Trimmed preset scope; non-strings and blanks are treated as global. */
function trimmedScope(model: unknown): string {
  return typeof model === 'string' ? model.trim() : ''
}

/** Case-insensitive comparison key for a preset scope. */
function scopeKey(model: unknown): string {
  return trimmedScope(model).toLowerCase()
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
      const model = trimmedScope(item.model)
      const preset: ModelWeightPreset = {
        name,
        weights: toOverrideList(item.weights),
      }
      if (model !== '') preset.model = model
      return [preset]
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
    presets.map((preset) => {
      const model = trimmedScope(preset.model)
      if (model === '') {
        return {
          name: preset.name.trim(),
          weights: serializeEntries(preset.weights),
        }
      }
      return {
        name: preset.name.trim(),
        model,
        weights: serializeEntries(preset.weights),
      }
    })
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

function entryKey(entry: ModelWeightOverrideEntry): string {
  return `${entry.channel_id}|${entry.model.trim().toLowerCase()}`
}

/** Drop repeated (channel_id, model) pairs, keeping the first occurrence. */
function dedupeEntries(
  entries: ModelWeightOverrideEntry[]
): ModelWeightOverrideEntry[] {
  const seen = new Set<string>()
  const result: ModelWeightOverrideEntry[] = []
  for (const entry of entries) {
    const key = entryKey(entry)
    if (seen.has(key)) continue
    seen.add(key)
    result.push(entry)
  }
  return result
}

function entriesForModel(
  entries: ModelWeightOverrideEntry[],
  model: string
): ModelWeightOverrideEntry[] {
  const key = scopeKey(model)
  if (key === '') return []
  return entries.filter((entry) => scopeKey(entry.model) === key)
}

/** Scoped presets that target `model`; global presets are never returned. */
export function presetsForModel(
  presets: ModelWeightPreset[],
  model: string
): ModelWeightPreset[] {
  const key = scopeKey(model)
  if (key === '') return []
  return presets.filter((preset) => scopeKey(preset.model) === key)
}

/** Name of the scoped preset matching `model`'s rows, or null for custom. */
export function findActiveScopedPresetName(
  presets: ModelWeightPreset[],
  model: string,
  weightsRaw: string
): string | null {
  const scoped = presetsForModel(presets, model)
  if (scoped.length === 0) return null

  const target = canonicalKey(
    entriesForModel(parseModelWeightEntries(weightsRaw), model)
  )
  for (const preset of scoped) {
    if (canonicalKey(entriesForModel(preset.weights, model)) === target) {
      return preset.name
    }
  }
  return null
}

/**
 * Merge a preset into the live weights.
 *
 * A global preset replaces the whole table. A scoped preset replaces only its
 * model's rows, inserted where that model's first row used to be (or appended
 * when the model has no rows yet); every other model keeps its order.
 */
export function applyPresetToModel(
  currentWeightsRaw: string,
  preset: ModelWeightPreset
): string {
  const scope = trimmedScope(preset.model)
  if (scope === '') {
    return JSON.stringify(serializeEntries(dedupeEntries(preset.weights)))
  }

  const current = parseModelWeightEntries(currentWeightsRaw)
  const key = scopeKey(scope)
  const firstIndex = current.findIndex((entry) => scopeKey(entry.model) === key)
  const others = current.filter((entry) => scopeKey(entry.model) !== key)
  const merged =
    firstIndex === -1
      ? [...others, ...preset.weights]
      : [
          ...others.slice(0, firstIndex),
          ...preset.weights,
          ...others.slice(firstIndex),
        ]
  return JSON.stringify(serializeEntries(dedupeEntries(merged)))
}

const MAX_MODEL_WEIGHT_VALUE = 1000000

/**
 * Convert within-tier percentages into integer weights. Non-positive and
 * non-finite shares are excluded (0); any positive share stays at least 1.
 */
export function normalizeRatioWeights(percents: number[]): number[] {
  return percents.map((percent) => {
    if (!Number.isFinite(percent) || percent <= 0) return 0
    return Math.min(MAX_MODEL_WEIGHT_VALUE, Math.max(1, Math.round(percent)))
  })
}
