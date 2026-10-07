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
import { parseModelsList } from '@/features/channels/lib/channel-utils'
import { extractMappingSourceModels } from '@/features/channels/lib/model-mapping-validation'
import type { Channel } from '@/features/channels/types'

import {
  normalizeRatioWeights,
  type ModelWeightOverrideEntry,
  type ModelWeightPreset,
} from './model-weight-presets'

export const MAX_MODEL_WEIGHT_VALUE = 1000000
export const MAX_MODEL_PRIORITY_VALUE = 1000000000

export type ModelAllocationRow = {
  /** Stable identity derived from (model, channel_id). */
  key: string
  channelId: number
  model: string
  channelName: string
  channelPriority: number
  channelWeight: number
  /** Raw override priority text; empty falls back to the channel priority. */
  priority: string
  /** Editable percentage text. Percentage is the share. */
  percent: string
}

export type ModelAllocationTier = {
  priority: number
  /** True for the highest tier; lower tiers are fallbacks. */
  participates: boolean
  rows: ModelAllocationRow[]
  /**
   * Sum of the tier's editable percentages. Stored weights are relative, so a
   * tier that does not total 100 still routes proportionally; the card shows
   * the sum so a typo is visible instead of silent.
   */
  totalPercent: number
}

export type ModelAllocationCard = {
  key: string
  model: string
  channelCount: number
  tiers: ModelAllocationTier[]
}

export function modelKey(model: string): string {
  return model.trim().toLowerCase()
}

export function rowKey(model: string, channelId: number): string {
  return `${modelKey(model)}|${channelId}`
}

/** True when the channel's model list or mapping source names `model`. */
export function channelServesModel(channel: Channel, model: string): boolean {
  const key = modelKey(model)
  if (key === '') return false
  if (parseModelsList(channel.models).some((item) => modelKey(item) === key)) {
    return true
  }
  return extractMappingSourceModels(channel.model_mapping ?? '').some(
    (item) => modelKey(item) === key
  )
}

export function effectivePriority(row: ModelAllocationRow): number {
  if (row.priority.trim() !== '') {
    const parsed = Number(row.priority)
    if (Number.isFinite(parsed)) return parsed
  }
  return row.channelPriority
}

/** Stored weight derived from a percentage: 0 stays 0, positives clamp to >= 1. */
export function derivedWeightFromPercent(percent: string): number {
  return normalizeRatioWeights([Number(percent)])[0]
}

/** Serialize one card row back into the override entry shape. */
export function rowToEntry(row: ModelAllocationRow): ModelWeightOverrideEntry {
  const entry: ModelWeightOverrideEntry = {
    channel_id: row.channelId,
    model: row.model,
    weight: derivedWeightFromPercent(row.percent),
  }
  if (row.priority.trim() !== '') {
    const parsed = Number(row.priority)
    if (Number.isFinite(parsed)) {
      entry.priority = Math.min(
        Math.max(Math.trunc(parsed), 0),
        MAX_MODEL_PRIORITY_VALUE
      )
    }
  }
  return entry
}

function equalPercents(count: number): string[] {
  if (count <= 0) return []
  const base = Math.floor(100 / count)
  const remainder = 100 - base * count
  return Array.from({ length: count }, (_, index) =>
    String(base + (index < remainder ? 1 : 0))
  )
}

/**
 * Turn a tier's effective weights into integer percentages that sum to 100.
 * An unknown or all-zero tier falls back to an equal split, matching the
 * backend's `sumWeight == 0` behaviour.
 */
function distributePercents(weights: number[]): string[] {
  if (weights.length === 0) return []
  const usable = weights.every((weight) => Number.isFinite(weight))
  const sum = usable
    ? weights.reduce((total, weight) => total + Math.max(0, weight), 0)
    : 0
  if (!usable || sum <= 0) return equalPercents(weights.length)

  const exact = weights.map((weight) => (Math.max(0, weight) / sum) * 100)
  const floors = exact.map((value) => Math.floor(value))
  const remainder = 100 - floors.reduce((total, value) => total + value, 0)
  const order = exact
    .map((value, index) => ({ index, fraction: value - Math.floor(value) }))
    .sort((left, right) => right.fraction - left.fraction || left.index - right.index)
  const result = [...floors]
  for (let step = 0; step < remainder; step += 1) {
    result[order[step % result.length].index] += 1
  }
  return result.map(String)
}

/** Allocate `total` across `count` rows, keeping the sum exact for integers. */
function distributeAmount(count: number, total: number): string[] {
  if (count <= 0) return []
  if (Number.isInteger(total)) {
    const base = Math.floor(total / count)
    const remainder = total - base * count
    return Array.from({ length: count }, (_, index) =>
      String(base + (index < remainder ? 1 : 0))
    )
  }
  const each = total / count
  return Array.from({ length: count }, () => String(Number(each.toFixed(2))))
}

function groupByPriority(
  rows: ModelAllocationRow[]
): Map<number, ModelAllocationRow[]> {
  const byPriority = new Map<number, ModelAllocationRow[]>()
  for (const row of rows) {
    const priority = effectivePriority(row)
    const list = byPriority.get(priority) ?? []
    list.push(row)
    byPriority.set(priority, list)
  }
  return byPriority
}

function sortRows(rows: ModelAllocationRow[]): ModelAllocationRow[] {
  return [...rows].sort(
    (left, right) =>
      effectivePriority(right) - effectivePriority(left) ||
      left.channelId - right.channelId
  )
}

function entriesByModel(
  entries: ModelWeightOverrideEntry[],
  model: string
): Map<number, ModelWeightOverrideEntry> {
  const key = modelKey(model)
  const byChannel = new Map<number, ModelWeightOverrideEntry>()
  for (const entry of entries) {
    if (modelKey(entry.model) !== key) continue
    if (!byChannel.has(entry.channel_id)) byChannel.set(entry.channel_id, entry)
  }
  return byChannel
}

/**
 * Build one editable row per enabled channel that serves the model. Channels
 * that no longer serve the model (including disabled ones) are never listed so
 * their existing override entries stay untouched.
 */
export function buildAllocationRows(
  entries: ModelWeightOverrideEntry[],
  channels: Channel[]
): ModelAllocationRow[] {
  const modelOrder: string[] = []
  const seenModels = new Set<string>()
  for (const entry of entries) {
    const model = entry.model.trim()
    if (model === '') continue
    const key = modelKey(model)
    if (seenModels.has(key)) continue
    seenModels.add(key)
    modelOrder.push(model)
  }

  const result: ModelAllocationRow[] = []
  for (const model of modelOrder) {
    const byChannel = entriesByModel(entries, model)
    const rows: ModelAllocationRow[] = []
    for (const channel of channels) {
      if (channel.status !== 1) continue
      if (!channelServesModel(channel, model)) continue
      const override = byChannel.get(channel.id)
      rows.push({
        key: rowKey(model, channel.id),
        channelId: channel.id,
        model,
        channelName: channel.name || `#${channel.id}`,
        channelPriority: channel.priority ?? 0,
        channelWeight: channel.weight ?? 0,
        priority:
          override?.priority !== undefined ? String(override.priority) : '',
        percent: '',
      })
    }
    for (const tierRows of groupByPriority(rows).values()) {
      const weights = tierRows.map((row) => {
        const override = byChannel.get(row.channelId)
        return override?.weight !== undefined && Number.isFinite(override.weight)
          ? override.weight
          : row.channelWeight
      })
      const percents = distributePercents(weights)
      tierRows.forEach((row, index) => {
        row.percent = percents[index]
      })
    }
    result.push(...sortRows(rows))
  }
  return result
}

/**
 * Group editable rows into one card per model and one tier per priority.
 * `models` seeds the card list so a model whose channels are all disabled or
 * gone still renders an empty card instead of disappearing.
 */
export function groupAllocationCards(
  rows: ModelAllocationRow[],
  models: string[]
): ModelAllocationCard[] {
  const order: string[] = []
  const displayByKey = new Map<string, string>()
  for (const model of models) {
    const key = modelKey(model)
    if (key === '' || displayByKey.has(key)) continue
    displayByKey.set(key, model.trim())
    order.push(key)
  }
  const byModel = new Map<string, ModelAllocationRow[]>()
  for (const row of rows) {
    const key = modelKey(row.model)
    if (!displayByKey.has(key)) {
      displayByKey.set(key, row.model)
      order.push(key)
    }
    const list = byModel.get(key) ?? []
    list.push(row)
    byModel.set(key, list)
  }

  return order.map((key) => {
    const modelRows = sortRows(byModel.get(key) ?? [])
    const tiers: ModelAllocationTier[] = []
    for (const row of modelRows) {
      const priority = effectivePriority(row)
      let tier = tiers.at(-1)
      if (!tier || tier.priority !== priority) {
        tier = {
          priority,
          participates: tiers.length === 0,
          rows: [],
          totalPercent: 0,
        }
        tiers.push(tier)
      }
      tier.rows.push(row)
    }
    for (const tier of tiers) {
      tier.totalPercent = tier.rows.reduce((sum, row) => {
        const value = Number(row.percent)
        return sum + (Number.isFinite(value) ? value : 0)
      }, 0)
    }
    return {
      key,
      model: displayByKey.get(key) ?? '',
      channelCount: modelRows.length,
      tiers,
    }
  })
}

/** Replace one model's editable rows. */
function replaceModelRows(
  rows: ModelAllocationRow[],
  replacements: Map<string, ModelAllocationRow>
): ModelAllocationRow[] {
  return rows.map((row) => replacements.get(row.key) ?? row)
}

/** Average split: every tier of the model gets equal percentages. */
export function averageSplitRows(
  rows: ModelAllocationRow[],
  model: string
): ModelAllocationRow[] {
  const key = modelKey(model)
  const modelRows = rows.filter((row) => modelKey(row.model) === key)
  const replacements = new Map<string, ModelAllocationRow>()
  for (const tierRows of groupByPriority(modelRows).values()) {
    const percents = equalPercents(tierRows.length)
    tierRows.forEach((row, index) => {
      replacements.set(row.key, { ...row, percent: percents[index] })
    })
  }
  return replaceModelRows(rows, replacements)
}

/**
 * Fill a model's percentages from a scoped preset without writing anything.
 * Rows the preset does not mention fall back to the channel weight, matching
 * the backend's fallback when no override exists.
 */
export function applyScopedPresetToRows(
  rows: ModelAllocationRow[],
  model: string,
  preset: ModelWeightPreset
): ModelAllocationRow[] {
  const key = modelKey(model)
  const byChannel = new Map<number, ModelWeightOverrideEntry>()
  for (const entry of preset.weights) {
    if (modelKey(entry.model) === key) byChannel.set(entry.channel_id, entry)
  }

  const modelRows = rows
    .filter((row) => modelKey(row.model) === key)
    .map((row) => {
      const entry = byChannel.get(row.channelId)
      return {
        ...row,
        priority: entry?.priority !== undefined ? String(entry.priority) : '',
      }
    })

  const replacements = new Map<string, ModelAllocationRow>()
  for (const tierRows of groupByPriority(modelRows).values()) {
    const weights = tierRows.map((row) => {
      const entry = byChannel.get(row.channelId)
      return entry?.weight !== undefined && Number.isFinite(entry.weight)
        ? entry.weight
        : row.channelWeight
    })
    const percents = distributePercents(weights)
    tierRows.forEach((row, index) => {
      replacements.set(row.key, { ...row, percent: percents[index] })
    })
  }
  return replaceModelRows(rows, replacements)
}

/** Percentage edit keeps the tier sum at 100 by splitting the remainder evenly. */
export function setRowPercent(
  rows: ModelAllocationRow[],
  key: string,
  value: string
): ModelAllocationRow[] {
  const target = rows.find((row) => row.key === key)
  if (!target) return rows
  const priority = effectivePriority(target)
  const others = rows.filter(
    (row) =>
      row.model === target.model &&
      row.key !== key &&
      effectivePriority(row) === priority
  )
  if (others.length === 0) {
    return rows.map((row) => (row.key === key ? { ...row, percent: value } : row))
  }
  const parsed = Number(value)
  const remaining = Number.isFinite(parsed)
    ? Math.min(100, Math.max(0, 100 - parsed))
    : 100
  const shares = distributeAmount(others.length, remaining)
  const byKey = new Map<string, string>()
  others.forEach((row, index) => byKey.set(row.key, shares[index]))
  return rows.map((row) => {
    if (row.key === key) return { ...row, percent: value }
    const share = byKey.get(row.key)
    return share === undefined ? row : { ...row, percent: share }
  })
}

/** Priority edit re-normalizes the affected tiers so each still sums to 100. */
export function setRowPriority(
  rows: ModelAllocationRow[],
  key: string,
  value: string
): ModelAllocationRow[] {
  const updated = rows.map((row) =>
    row.key === key ? { ...row, priority: value } : row
  )
  const target = updated.find((row) => row.key === key)
  if (!target) return updated
  const modelRows = updated.filter((row) => row.model === target.model)
  const replacements = new Map<string, ModelAllocationRow>()
  for (const tierRows of groupByPriority(modelRows).values()) {
    const weights = tierRows.map((row) => Number(row.percent))
    const percents = distributePercents(weights)
    tierRows.forEach((row, index) => {
      replacements.set(row.key, { ...row, percent: percents[index] })
    })
  }
  return replaceModelRows(updated, replacements)
}

/** Canonical draft state used for dirty tracking. */
export function serializeDraftState(rows: ModelAllocationRow[]): string {
  return JSON.stringify(
    [...rows]
      .sort(
        (left, right) =>
          modelKey(left.model).localeCompare(modelKey(right.model)) ||
          left.channelId - right.channelId
      )
      .map((row) => [
        modelKey(row.model),
        row.channelId,
        effectivePriority(row),
        row.priority.trim(),
        row.percent.trim(),
      ])
  )
}

/** Model keys whose editable draft differs from the saved baseline. */
export function computeDirtyModelKeys(
  rows: ModelAllocationRow[],
  baselineRows: ModelAllocationRow[]
): Set<string> {
  const keys = new Set<string>()
  const models = new Set(
    [...rows, ...baselineRows].map((row) => modelKey(row.model))
  )
  for (const key of models) {
    const draft = serializeDraftState(
      rows.filter((row) => modelKey(row.model) === key)
    )
    const baseline = serializeDraftState(
      baselineRows.filter((row) => modelKey(row.model) === key)
    )
    if (draft !== baseline) keys.add(key)
  }
  return keys
}

/**
 * Serialize the draft table. Only dirty models are rewritten; every entry of a
 * clean model (and every entry whose channel is not shown) is preserved
 * byte-identical.
 */
export function serializeAllocationDraft(
  baseEntries: ModelWeightOverrideEntry[],
  rows: ModelAllocationRow[],
  dirtyModelKeys: ReadonlySet<string>
): string {
  const rowByKey = new Map(
    rows.map((row) => [rowKey(row.model, row.channelId), row])
  )
  const emitted = new Set<string>()
  const result: ModelWeightOverrideEntry[] = []

  for (const entry of baseEntries) {
    const key = rowKey(entry.model, entry.channel_id)
    const row = rowByKey.get(key)
    if (row && dirtyModelKeys.has(modelKey(entry.model))) {
      if (emitted.has(key)) continue
      emitted.add(key)
      result.push(rowToEntry(row))
      continue
    }
    result.push(entry)
    emitted.add(key)
  }

  for (const row of rows) {
    const key = rowKey(row.model, row.channelId)
    if (emitted.has(key)) continue
    if (!dirtyModelKeys.has(modelKey(row.model))) continue
    emitted.add(key)
    result.push(rowToEntry(row))
  }

  return JSON.stringify(result)
}
