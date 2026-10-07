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
import type { Channel } from '@/features/channels/types'
import { formatPercent } from '@/lib/format'

import {
  applyPresetToModel,
  normalizeRatioWeights,
  parseModelWeightEntries,
  type ModelWeightOverrideEntry,
} from './model-weight-presets'

export const MAX_MODEL_WEIGHT_VALUE = 1000000
export const MAX_MODEL_PRIORITY_VALUE = 1000000000
export const CUSTOM_RATIO_VALUE = '__custom_ratio__'

export type ModelWeightChannelOption = {
  label: string
  value: string
}

export type ModelWeightRow = {
  /** Client-side stable identity for React keys; stripped before saving. */
  key: string
  channel_id: number
  model: string
  /** Empty means "no override"; an explicit 0 is a real value. */
  weight: string
  /** Empty means "no override"; an explicit 0 is a real value. */
  priority: string
}

export type GroupedRow = {
  row: ModelWeightRow
  index: number
  tier: number
  weight: number | undefined
}

export type RowGroup = {
  model: string
  key: string
  rows: GroupedRow[]
}

export type CustomRatioTier = {
  tier: number
  entries: ModelWeightOverrideEntry[]
}

export type CustomRatioState = {
  model: string
  tiers: CustomRatioTier[]
  percents: Record<string, string>
}

let modelWeightRowKeySeq = 0

export const nextModelWeightRowKey = () =>
  `model-weight-${++modelWeightRowKeySeq}`

function modelKey(model: string) {
  return model.trim().toLowerCase()
}

export function isSameModel(left: string, right: string) {
  return modelKey(left) === modelKey(right)
}

export function entryKey(entry: ModelWeightOverrideEntry) {
  return `${entry.channel_id}|${modelKey(entry.model)}`
}

export function effectiveRowPriority(row: ModelWeightRow, channel?: Channel) {
  if (row.priority.trim() !== '') {
    const parsed = Number(row.priority)
    if (Number.isFinite(parsed)) return parsed
  }
  return channel?.priority ?? 0
}

export function effectiveRowWeight(
  row: ModelWeightRow,
  channel?: Channel
): number | undefined {
  if (row.weight.trim() !== '') {
    const parsed = Number(row.weight)
    if (Number.isFinite(parsed)) return parsed
  }
  return channel?.weight ?? undefined
}

function effectiveEntryPriority(
  entry: ModelWeightOverrideEntry,
  channel?: Channel
) {
  if (entry.priority !== undefined && Number.isFinite(entry.priority)) {
    return entry.priority
  }
  return channel?.priority ?? 0
}

function effectiveEntryWeight(
  entry: ModelWeightOverrideEntry,
  channel?: Channel
): number | undefined {
  if (entry.weight !== undefined && Number.isFinite(entry.weight)) {
    return entry.weight
  }
  return channel?.weight ?? undefined
}

function optionalNumberText(value: unknown) {
  return value === undefined || value === null ? '' : String(value)
}

export function parseModelWeights(value: string): ModelWeightRow[] {
  try {
    const parsed: unknown = JSON.parse(value || '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.map((item) => {
      const record = (item ?? {}) as Record<string, unknown>
      return {
        key: nextModelWeightRowKey(),
        channel_id: Number(record.channel_id) || 0,
        model: typeof record.model === 'string' ? record.model : '',
        weight: optionalNumberText(record.weight),
        priority: optionalNumberText(record.priority),
      }
    })
  } catch {
    return []
  }
}

export function serializeModelWeights(rows: ModelWeightRow[]) {
  const entries = rows
    .filter((row) => row.channel_id > 0 && row.model.trim() !== '')
    .map((row) => {
      const entry: Record<string, number | string> = {
        channel_id: row.channel_id,
        model: row.model.trim(),
      }
      if (row.weight.trim() !== '') {
        entry.weight = Math.min(
          Math.max(Number(row.weight) || 0, 0),
          MAX_MODEL_WEIGHT_VALUE
        )
      }
      if (row.priority.trim() !== '') {
        entry.priority = Math.min(
          Math.max(Number(row.priority) || 0, 0),
          MAX_MODEL_PRIORITY_VALUE
        )
      }
      return entry
    })
    // A row with neither value would be rejected by the backend validator.
    .filter((entry) => 'weight' in entry || 'priority' in entry)
  return JSON.stringify(entries)
}

/** Replace one model's editor rows, keeping every other model (and draft) row. */
export function replaceModelRows(
  current: ModelWeightRow[],
  model: string,
  mergedWeights: string
): ModelWeightRow[] {
  const replacement = parseModelWeights(mergedWeights).filter((row) =>
    isSameModel(row.model, model)
  )
  const firstIndex = current.findIndex((row) => isSameModel(row.model, model))
  const others = current.filter((row) => !isSameModel(row.model, model))
  if (firstIndex === -1) return [...others, ...replacement]
  return [
    ...others.slice(0, firstIndex),
    ...replacement,
    ...others.slice(firstIndex),
  ]
}

function groupEntriesByTier(
  entries: ModelWeightOverrideEntry[],
  channelById: Map<number, Channel>
) {
  const tiers = new Map<number, ModelWeightOverrideEntry[]>()
  for (const entry of entries) {
    const tier = effectiveEntryPriority(
      entry,
      channelById.get(entry.channel_id)
    )
    const list = tiers.get(tier) ?? []
    list.push(entry)
    tiers.set(tier, list)
  }
  for (const list of tiers.values()) {
    list.sort((left, right) => left.channel_id - right.channel_id)
  }
  return tiers
}

export function tierTotal(
  tier: CustomRatioTier,
  percents: Record<string, string>
) {
  return tier.entries.reduce((total, entry) => {
    const value = Number(percents[entryKey(entry)] ?? '')
    return total + (Number.isFinite(value) ? value : 0)
  }, 0)
}

export function buildCustomRatioState(
  savedWeights: string,
  model: string,
  channelById: Map<number, Channel>
): CustomRatioState {
  const entries = parseModelWeightEntries(savedWeights).filter((entry) =>
    isSameModel(entry.model, model)
  )
  const tiers = [...groupEntriesByTier(entries, channelById).entries()]
    .sort((left, right) => right[0] - left[0])
    .map(([tier, tierEntries]) => ({ tier, entries: tierEntries }))

  const percents: Record<string, string> = {}
  for (const tier of tiers) {
    const weights = tier.entries.map((entry) =>
      effectiveEntryWeight(entry, channelById.get(entry.channel_id))
    )
    const resolved = weights.every(
      (weight) => weight !== undefined && Number.isFinite(weight)
    )
    const sum = resolved
      ? weights.reduce<number>((total, weight) => total + (weight as number), 0)
      : 0
    tier.entries.forEach((entry, index) => {
      const weight = weights[index]
      if (!resolved || sum <= 0 || weight === undefined || weight <= 0) {
        percents[entryKey(entry)] = '0'
        return
      }
      percents[entryKey(entry)] = String(
        Math.max(1, Math.round((weight / sum) * 100))
      )
    })
  }

  return { model, tiers, percents }
}

/** Merge the custom-ratio editor percentages back into the saved weights. */
export function applyCustomRatioWeights(
  savedWeights: string,
  state: CustomRatioState
): string {
  const { model, tiers, percents } = state
  const weightByEntryKey = new Map<string, number>()
  for (const tier of tiers) {
    const values = tier.entries.map((entry) => {
      const parsed = Number(percents[entryKey(entry)] ?? '')
      return Number.isFinite(parsed) ? parsed : 0
    })
    const weights = normalizeRatioWeights(values)
    tier.entries.forEach((entry, index) => {
      weightByEntryKey.set(entryKey(entry), weights[index])
    })
  }
  const modelEntries = parseModelWeightEntries(savedWeights).filter((entry) =>
    isSameModel(entry.model, model)
  )
  const updated = modelEntries.map((entry) => {
    const weight = weightByEntryKey.get(entryKey(entry))
    return weight === undefined ? entry : { ...entry, weight }
  })
  return applyPresetToModel(savedWeights, {
    name: '',
    model,
    weights: updated,
  })
}

export function groupRowsByModel(
  rows: ModelWeightRow[],
  channelById: Map<number, Channel>
): RowGroup[] {
  const order: string[] = []
  const byModel = new Map<string, GroupedRow[]>()
  rows.forEach((row, index) => {
    const model = row.model.trim()
    if (model === '') return
    const key = modelKey(model)
    let list = byModel.get(key)
    if (!list) {
      list = []
      byModel.set(key, list)
      order.push(key)
    }
    const channel = channelById.get(row.channel_id)
    list.push({
      row,
      index,
      tier: effectiveRowPriority(row, channel),
      weight: effectiveRowWeight(row, channel),
    })
  })
  return order.flatMap((key) => {
    const list = byModel.get(key)
    if (!list || list.length === 0) return []
    list.sort(
      (left, right) =>
        right.tier - left.tier || left.row.channel_id - right.row.channel_id
    )
    return [{ model: list[0].row.model.trim(), key, rows: list }]
  })
}

/** Draft rows (no model yet) render outside any model group. */
export function buildDraftRows(rows: ModelWeightRow[]) {
  return rows
    .map((row, index) => ({ row, index }))
    .filter(({ row }) => row.model.trim() === '')
}

/** Known channels plus fallbacks for channel IDs that no longer resolve. */
export function buildChannelOptions(
  channels: Channel[] | undefined,
  rows: ModelWeightRow[]
): ModelWeightChannelOption[] {
  const options = new Map(
    (channels ?? []).map((channel) => [
      channel.id,
      {
        label: `#${channel.id} - ${channel.name}`,
        value: String(channel.id),
      },
    ])
  )
  for (const row of rows) {
    if (row.channel_id > 0 && !options.has(row.channel_id)) {
      options.set(row.channel_id, {
        label: `#${row.channel_id}`,
        value: String(row.channel_id),
      })
    }
  }
  return [...options.values()].sort(
    (left, right) => Number(left.value) - Number(right.value)
  )
}

export function computeShareByGroupKey(groups: RowGroup[]) {
  const result = new Map<string, Map<string, string>>()
  for (const group of groups) {
    const byRowKey = new Map<string, string>()
    const byTier = new Map<number, GroupedRow[]>()
    for (const grouped of group.rows) {
      const list = byTier.get(grouped.tier) ?? []
      list.push(grouped)
      byTier.set(grouped.tier, list)
    }
    for (const list of byTier.values()) {
      const resolved = list.every(
        (grouped) =>
          grouped.weight !== undefined && Number.isFinite(grouped.weight)
      )
      const sum = resolved
        ? list.reduce<number>(
            (total, grouped) => total + (grouped.weight as number),
            0
          )
        : 0
      for (const grouped of list) {
        if (!resolved || sum <= 0) {
          byRowKey.set(grouped.row.key, '-')
          continue
        }
        byRowKey.set(
          grouped.row.key,
          formatPercent(Math.round(((grouped.weight as number) / sum) * 100))
        )
      }
    }
    result.set(group.key, byRowKey)
  }
  return result
}

export function computeUnsavedByGroupKey(
  groups: RowGroup[],
  rows: ModelWeightRow[],
  savedWeights: string
) {
  const savedRows = parseModelWeights(savedWeights)
  const result = new Map<string, boolean>()
  for (const group of groups) {
    const editorSerialized = serializeModelWeights(
      rows.filter((row) => isSameModel(row.model, group.model))
    )
    const savedSerialized = serializeModelWeights(
      savedRows.filter((row) => isSameModel(row.model, group.model))
    )
    result.set(group.key, editorSerialized !== savedSerialized)
  }
  return result
}
