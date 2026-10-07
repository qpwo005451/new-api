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
import { useQuery } from '@tanstack/react-query'
import { Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getChannels } from '@/features/channels/api'
import type { Channel } from '@/features/channels/types'
import { formatPercent } from '@/lib/format'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSavePolicy } from '../request-policies/use-save-policy'
import { ModelWeightPresetBar } from './model-weight-preset-bar'
import {
  applyPresetToModel,
  findActiveScopedPresetName,
  MODEL_WEIGHTS_OPTION_KEY,
  MODEL_WEIGHT_PRESETS_OPTION_KEY,
  normalizeRatioWeights,
  parseModelWeightEntries,
  parseModelWeightPresets,
  presetsForModel,
  type ModelWeightOverrideEntry,
  type ModelWeightPreset,
} from './model-weight-presets'

const MODEL_WEIGHTS_KEY = MODEL_WEIGHTS_OPTION_KEY
const MODEL_WEIGHTS_PRESETS_KEY = MODEL_WEIGHT_PRESETS_OPTION_KEY
const CHANNEL_PAGE_SIZE = 100
const MAX_MODEL_WEIGHT_VALUE = 1000000
const MAX_MODEL_PRIORITY_VALUE = 1000000000
const CUSTOM_RATIO_VALUE = '__custom_ratio__'

type ModelWeightRow = {
  /** Client-side stable identity for React keys; stripped before saving. */
  key: string
  channel_id: number
  model: string
  /** Empty means "no override"; an explicit 0 is a real value. */
  weight: string
  /** Empty means "no override"; an explicit 0 is a real value. */
  priority: string
}

type Props = {
  defaultValues: {
    'model_weight_setting.weights': string
    'model_weight_setting.presets': string
  }
}

type GroupedRow = {
  row: ModelWeightRow
  index: number
  tier: number
  weight: number | undefined
}

type CustomRatioTier = {
  tier: number
  entries: ModelWeightOverrideEntry[]
}

type CustomRatioState = {
  model: string
  tiers: CustomRatioTier[]
  percents: Record<string, string>
}

let modelWeightRowKeySeq = 0

const nextModelWeightRowKey = () => `model-weight-${++modelWeightRowKeySeq}`

function modelKey(model: string) {
  return model.trim().toLowerCase()
}

function isSameModel(left: string, right: string) {
  return modelKey(left) === modelKey(right)
}

function entryKey(entry: ModelWeightOverrideEntry) {
  return `${entry.channel_id}|${modelKey(entry.model)}`
}

function effectiveRowPriority(row: ModelWeightRow, channel?: Channel) {
  if (row.priority.trim() !== '') {
    const parsed = Number(row.priority)
    if (Number.isFinite(parsed)) return parsed
  }
  return channel?.priority ?? 0
}

function effectiveRowWeight(
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

async function fetchAllChannels() {
  const firstResponse = await getChannels({
    p: 1,
    page_size: CHANNEL_PAGE_SIZE,
    id_sort: true,
    sort_by: 'id',
    sort_order: 'asc',
  })
  if (!firstResponse.success || !firstResponse.data) {
    throw new Error(firstResponse.message || 'Failed to load channels')
  }

  const totalPages = Math.ceil(
    firstResponse.data.total /
      Math.max(firstResponse.data.page_size, CHANNEL_PAGE_SIZE)
  )
  const remainingResponses = await Promise.all(
    Array.from({ length: Math.max(0, totalPages - 1) }, (_, index) =>
      getChannels({
        p: index + 2,
        page_size: CHANNEL_PAGE_SIZE,
        id_sort: true,
        sort_by: 'id',
        sort_order: 'asc',
      })
    )
  )
  for (const response of remainingResponses) {
    if (!response.success || !response.data) {
      throw new Error(response.message || 'Failed to load channels')
    }
  }

  return [firstResponse, ...remainingResponses].flatMap(
    (response) => response.data?.items ?? []
  )
}

function optionalNumberText(value: unknown) {
  return value === undefined || value === null ? '' : String(value)
}

function parseModelWeights(value: string): ModelWeightRow[] {
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

function serializeModelWeights(rows: ModelWeightRow[]) {
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
function replaceModelRows(
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

function tierTotal(tier: CustomRatioTier, percents: Record<string, string>) {
  return tier.entries.reduce((total, entry) => {
    const value = Number(percents[entryKey(entry)] ?? '')
    return total + (Number.isFinite(value) ? value : 0)
  }, 0)
}

function buildCustomRatioState(
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

export function ModelWeightSection(props: Props) {
  const { t } = useTranslation()
  const savePolicy = useSavePolicy()
  const initialRows = useMemo(
    () => parseModelWeights(props.defaultValues[MODEL_WEIGHTS_KEY]),
    [props.defaultValues]
  )
  const [rows, setRows] = useState<ModelWeightRow[]>(initialRows)
  const [savedWeights, setSavedWeights] = useState(() =>
    serializeModelWeights(initialRows)
  )
  const baselineRef = useRef(serializeModelWeights(initialRows))
  const [pendingApply, setPendingApply] = useState<{
    model: string
    preset: ModelWeightPreset
  } | null>(null)
  const [customRatio, setCustomRatio] = useState<CustomRatioState | null>(null)

  useEffect(() => {
    const parsed = parseModelWeights(props.defaultValues[MODEL_WEIGHTS_KEY])
    const serialized = serializeModelWeights(parsed)
    setRows(parsed)
    setSavedWeights(serialized)
    baselineRef.current = serialized
  }, [props.defaultValues])

  const channelsQuery = useQuery({
    queryKey: ['system-settings', 'model-weights', 'channels'],
    queryFn: fetchAllChannels,
    retry: false,
  })

  const channelById = useMemo(
    () =>
      new Map(
        (channelsQuery.data ?? []).map((channel) => [channel.id, channel])
      ),
    [channelsQuery.data]
  )

  const presets = useMemo(
    () =>
      parseModelWeightPresets(props.defaultValues[MODEL_WEIGHTS_PRESETS_KEY]),
    [props.defaultValues]
  )

  // Known channels plus fallback entries so IDs that no longer resolve
  // (e.g. deleted channels) still render as selectable options.
  const channelOptions = useMemo(() => {
    const options = new Map(
      (channelsQuery.data ?? []).map((channel) => [
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
  }, [channelsQuery.data, rows])

  const groups = useMemo(() => {
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
  }, [rows, channelById])

  const draftRows = useMemo(
    () =>
      rows
        .map((row, index) => ({ row, index }))
        .filter(({ row }) => row.model.trim() === ''),
    [rows]
  )

  const shareByGroupKey = useMemo(() => {
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
  }, [groups])

  const unsavedByGroupKey = useMemo(() => {
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
  }, [groups, rows, savedWeights])

  const updateRow = (index: number, changes: Partial<ModelWeightRow>) => {
    setRows((prev) =>
      prev.map((row, i) => (i === index ? { ...row, ...changes } : row))
    )
  }

  const handleAdd = () => {
    setRows((prev) => [
      ...prev,
      {
        key: nextModelWeightRowKey(),
        channel_id: 0,
        model: '',
        weight: '',
        priority: '',
      },
    ])
  }

  const handleRemove = (index: number) => {
    setRows((prev) => prev.filter((_, i) => i !== index))
  }

  const handleSave = async () => {
    const serialized = serializeModelWeights(rows)
    if (serialized === baselineRef.current) {
      toast.info(t('No changes to save'))
      return
    }
    await savePolicy.mutateAsync({ [MODEL_WEIGHTS_KEY]: serialized })
    baselineRef.current = serialized
    setSavedWeights(serialized)
  }

  const handleRatioChange = (model: string, value: string) => {
    if (value === CUSTOM_RATIO_VALUE) {
      setCustomRatio(buildCustomRatioState(savedWeights, model, channelById))
      return
    }
    const preset = presetsForModel(presets, model).find(
      (candidate) => candidate.name === value
    )
    if (preset) setPendingApply({ model, preset })
  }

  const handleApplyPreset = async () => {
    if (!pendingApply) return
    const { model, preset } = pendingApply
    const merged = applyPresetToModel(savedWeights, preset)
    await savePolicy.mutateAsync({ [MODEL_WEIGHTS_KEY]: merged })
    baselineRef.current = merged
    setSavedWeights(merged)
    setRows((prev) => replaceModelRows(prev, model, merged))
    setPendingApply(null)
  }

  const handleCustomConfirm = async () => {
    if (!customRatio) return
    const { model, tiers, percents } = customRatio
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
    const merged = applyPresetToModel(savedWeights, {
      name: '',
      model,
      weights: updated,
    })
    await savePolicy.mutateAsync({ [MODEL_WEIGHTS_KEY]: merged })
    baselineRef.current = merged
    setSavedWeights(merged)
    setRows((prev) => replaceModelRows(prev, model, merged))
    setCustomRatio(null)
  }

  const hasPositiveCustomShare = customRatio
    ? Object.values(customRatio.percents).some((value) => Number(value) > 0)
    : false

  const renderRow = (row: ModelWeightRow, index: number, share: string) => (
    <div
      key={row.key}
      className='grid gap-3 rounded-lg border p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_7rem_7rem_5rem_auto]'
    >
      <label className='grid gap-1.5 text-sm'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Channel')}
        </span>
        <Select
          items={channelOptions}
          value={row.channel_id > 0 ? String(row.channel_id) : null}
          onValueChange={(value) =>
            value !== null && updateRow(index, { channel_id: Number(value) })
          }
        >
          <SelectTrigger aria-label={t('Channel')} className='w-full'>
            <SelectValue placeholder={t('Select a channel')} />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            {channelOptions.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </label>
      <label className='grid gap-1.5 text-sm'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Model')}
        </span>
        <Input
          aria-label={t('Model')}
          value={row.model}
          onChange={(event) => updateRow(index, { model: event.target.value })}
          placeholder='deepseek-v4.1-flash'
        />
      </label>
      <label className='grid gap-1.5 text-sm'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Weight')}
        </span>
        <Input
          aria-label={t('Weight')}
          type='number'
          min={0}
          max={MAX_MODEL_WEIGHT_VALUE}
          value={row.weight}
          onChange={(event) => updateRow(index, { weight: event.target.value })}
        />
      </label>
      <label className='grid gap-1.5 text-sm'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Priority')}
        </span>
        <Input
          aria-label={t('Priority')}
          type='number'
          min={0}
          max={MAX_MODEL_PRIORITY_VALUE}
          value={row.priority}
          onChange={(event) =>
            updateRow(index, { priority: event.target.value })
          }
        />
      </label>
      <div className='grid gap-1.5 text-sm'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Share')}
        </span>
        <span className='flex h-8 items-center text-sm'>{share}</span>
      </div>
      <div className='flex items-end justify-end'>
        <Button
          type='button'
          variant='outline'
          size='icon'
          aria-label={t('Remove')}
          onClick={() => handleRemove(index)}
        >
          <Trash2 />
        </Button>
      </div>
    </div>
  )

  return (
    <SettingsSection title={t('Model Routing')}>
      <p className='text-muted-foreground text-sm font-medium'>
        {t('Per-model channel routing')}
      </p>
      <p className='text-muted-foreground text-sm'>
        {t('Overrides the channel priority or weight for one model only.')}
      </p>
      <SettingsForm
        onSubmit={(event) => {
          event.preventDefault()
          void handleSave()
        }}
      >
        <SettingsPageFormActions
          onSave={() => void handleSave()}
          isSaving={savePolicy.isPending}
        />
        <ModelWeightPresetBar
          presets={presets}
          currentWeights={savedWeights}
          editorWeights={serializeModelWeights(rows)}
        />
        <div className='flex flex-wrap items-center gap-2'>
          <Button type='button' size='sm' onClick={handleAdd}>
            <Plus data-icon='inline-start' />
            {t('Add override')}
          </Button>
          <span className='text-muted-foreground text-xs'>
            {t('Unset fields keep the channel value.')}{' '}
            {t('A weight of 0 excludes the model on this channel.')}{' '}
            {t(
              'Priority decides which channels compete; weight splits traffic within one priority.'
            )}
          </span>
        </div>

        {rows.length === 0 ? (
          <div className='text-muted-foreground/80 rounded-lg border border-dashed px-5 py-8 text-center text-sm'>
            {t('No overrides yet. Click "Add override" to create one.')}
          </div>
        ) : (
          <div className='space-y-4'>
            {groups.map((group) => {
              const activeName = findActiveScopedPresetName(
                presets,
                group.model,
                savedWeights
              )
              const ratioItems = [
                ...presetsForModel(presets, group.model).map((preset) => ({
                  label: preset.name,
                  value: preset.name,
                })),
                { label: t('Custom ratio'), value: CUSTOM_RATIO_VALUE },
              ]
              const shares = shareByGroupKey.get(group.key)
              return (
                <div key={group.key} className='rounded-lg border'>
                  <div className='flex flex-wrap items-center justify-between gap-2 border-b p-3'>
                    <div className='flex items-center gap-2'>
                      <span className='text-sm font-medium'>{group.model}</span>
                      {unsavedByGroupKey.get(group.key) ? (
                        <span className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[0.7rem] font-medium'>
                          {t('Unsaved changes')}
                        </span>
                      ) : null}
                    </div>
                    <div className='flex items-center gap-2'>
                      <span className='text-muted-foreground text-xs font-medium'>
                        {t('Routing ratio')}
                      </span>
                      <Select
                        items={ratioItems}
                        value={activeName ?? CUSTOM_RATIO_VALUE}
                        onValueChange={(value) =>
                          value !== null &&
                          handleRatioChange(group.model, value)
                        }
                      >
                        <SelectTrigger
                          aria-label={`${t('Routing ratio')} ${group.model}`}
                          className='w-48'
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          {ratioItems.map((item) => (
                            <SelectItem key={item.value} value={item.value}>
                              {item.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>
                  <div className='space-y-3 p-3'>
                    {group.rows.map((grouped) =>
                      renderRow(
                        grouped.row,
                        grouped.index,
                        shares?.get(grouped.row.key) ?? '-'
                      )
                    )}
                  </div>
                </div>
              )
            })}
            {draftRows.length > 0 ? (
              <div className='space-y-3 rounded-lg border border-dashed p-3'>
                {draftRows.map(({ row, index }) => renderRow(row, index, '-'))}
              </div>
            ) : null}
          </div>
        )}

        <p className='text-muted-foreground text-xs'>
          {t('The share is computed inside one priority tier.')}
        </p>
      </SettingsForm>

      <ConfirmDialog
        open={pendingApply !== null}
        onOpenChange={(open) => {
          if (!open) setPendingApply(null)
        }}
        title={t('Apply preset to this model?')}
        desc={
          pendingApply
            ? t('This replaces the overrides of {{model}} only.', {
                model: pendingApply.model,
              })
            : ''
        }
        confirmText={t('Apply')}
        isLoading={savePolicy.isPending}
        handleConfirm={() => void handleApplyPreset()}
      />

      <Dialog
        open={customRatio !== null}
        onOpenChange={(open) => {
          if (!open) setCustomRatio(null)
        }}
        title={t('Custom ratio')}
        description={t(
          'Set the ratio as percentages. They are stored as integer weights.'
        )}
        footer={
          <>
            <Button
              type='button'
              variant='outline'
              onClick={() => setCustomRatio(null)}
            >
              {t('Cancel')}
            </Button>
            <Button
              type='button'
              disabled={!hasPositiveCustomShare || savePolicy.isPending}
              onClick={() => void handleCustomConfirm()}
            >
              {t('Apply')}
            </Button>
          </>
        }
      >
        {customRatio ? (
          <div className='space-y-4'>
            {customRatio.tiers.length === 0 ? (
              <p className='text-muted-foreground text-sm'>
                {t('No overrides yet. Click "Add override" to create one.')}
              </p>
            ) : (
              customRatio.tiers.map((tier) => (
                <div
                  key={tier.tier}
                  className='space-y-2 rounded-lg border p-3'
                >
                  <div className='flex items-center justify-between gap-2 text-xs'>
                    <span className='text-muted-foreground font-medium'>
                      {t('Priority')} {tier.tier}
                    </span>
                    <span className='text-muted-foreground'>
                      {t('Total')}: {tierTotal(tier, customRatio.percents)}%
                    </span>
                  </div>
                  {tier.entries.map((entry) => (
                    <label
                      key={entryKey(entry)}
                      className='flex items-center justify-between gap-3 text-sm'
                    >
                      <span>#{entry.channel_id}</span>
                      <Input
                        aria-label={`${t('Share')} #${entry.channel_id}`}
                        type='number'
                        min={0}
                        max={100}
                        value={customRatio.percents[entryKey(entry)] ?? ''}
                        onChange={(event) =>
                          setCustomRatio((prev) =>
                            prev
                              ? {
                                  ...prev,
                                  percents: {
                                    ...prev.percents,
                                    [entryKey(entry)]: event.target.value,
                                  },
                                }
                              : prev
                          )
                        }
                      />
                    </label>
                  ))}
                </div>
              ))
            )}
            {!hasPositiveCustomShare ? (
              <p className='text-destructive text-sm'>
                {t('Enter a ratio greater than 0.')}
              </p>
            ) : null}
          </div>
        ) : null}
      </Dialog>
    </SettingsSection>
  )
}
