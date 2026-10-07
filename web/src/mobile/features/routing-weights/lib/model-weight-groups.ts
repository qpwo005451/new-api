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
import type { ModelWeightOverrideEntry } from '@/features/system-settings/models/model-weight-presets'

export interface ChannelWeightInfo {
  name: string
  priority: number
  weight: number
}

export type ChannelWeightLookup = ReadonlyMap<number, ChannelWeightInfo>

export interface ModelWeightRowView {
  entry: ModelWeightOverrideEntry
  channel: ChannelWeightInfo | undefined
  priority: number
  weight: number | null
  /** Percent inside the row's priority tier; null when it cannot be trusted. */
  sharePercent: number | null
}

export interface ModelWeightGroup {
  model: string
  rows: ModelWeightRowView[]
  hasUnavailableShare: boolean
}

function modelKey(model: string): string {
  return model.trim().toLowerCase()
}

function effectivePriority(
  entry: ModelWeightOverrideEntry,
  channel: ChannelWeightInfo | undefined
): number {
  return entry.priority ?? channel?.priority ?? 0
}

function effectiveWeight(
  entry: ModelWeightOverrideEntry,
  channel: ChannelWeightInfo | undefined
): number | null {
  const weight = entry.weight ?? channel?.weight
  return typeof weight === 'number' && Number.isFinite(weight) ? weight : null
}

/**
 * Group the override rows by model (first-appearance order), order each group
 * by effective priority desc then channel id asc, and compute each row's share
 * inside its priority tier. A tier is left without a share when any of its
 * effective weights is unknown or the tier sums to zero, so the UI never shows
 * a guessed number.
 */
export function buildModelWeightGroups(
  entries: ModelWeightOverrideEntry[],
  channels: ChannelWeightLookup
): ModelWeightGroup[] {
  const order: string[] = []
  const entriesByModel = new Map<string, ModelWeightOverrideEntry[]>()
  for (const entry of entries) {
    const model = entry.model.trim()
    const key = modelKey(model)
    if (!entriesByModel.has(key)) {
      entriesByModel.set(key, [])
      order.push(model)
    }
    entriesByModel.get(key)?.push(entry)
  }

  return order.map((model) => {
    const sorted = [...(entriesByModel.get(modelKey(model)) ?? [])].sort(
      (left, right) => {
        const leftPriority = effectivePriority(
          left,
          channels.get(left.channel_id)
        )
        const rightPriority = effectivePriority(
          right,
          channels.get(right.channel_id)
        )
        if (leftPriority !== rightPriority) return rightPriority - leftPriority
        return left.channel_id - right.channel_id
      }
    )

    const tiers: ModelWeightRowView[][] = []
    let currentPriority: number | null = null
    for (const entry of sorted) {
      const channel = channels.get(entry.channel_id)
      const priority = effectivePriority(entry, channel)
      const view: ModelWeightRowView = {
        entry,
        channel,
        priority,
        weight: effectiveWeight(entry, channel),
        sharePercent: null,
      }
      if (tiers.length === 0 || priority !== currentPriority) {
        tiers.push([])
        currentPriority = priority
      }
      tiers.at(-1)?.push(view)
    }

    for (const tier of tiers) {
      const known = tier.every((row) => row.weight !== null)
      const sum = tier.reduce((total, row) => total + (row.weight ?? 0), 0)
      if (!known || sum <= 0) continue
      for (const row of tier) {
        row.sharePercent = Math.round(((row.weight ?? 0) / sum) * 100)
      }
    }

    const rows = tiers.flat()
    return {
      model,
      rows,
      hasUnavailableShare: rows.some((row) => row.sharePercent === null),
    }
  })
}

/** Replace the target model's weights without touching order or priority. */
export function applyNormalizedWeights(
  entries: ModelWeightOverrideEntry[],
  model: string,
  weightsByChannel: ReadonlyMap<number, number>
): ModelWeightOverrideEntry[] {
  const key = modelKey(model)
  return entries.map((entry) => {
    if (modelKey(entry.model) !== key) return entry
    const weight = weightsByChannel.get(entry.channel_id)
    return weight === undefined ? entry : { ...entry, weight }
  })
}
