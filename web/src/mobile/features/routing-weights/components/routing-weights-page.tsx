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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  MODEL_WEIGHT_PRESETS_OPTION_KEY,
  MODEL_WEIGHTS_OPTION_KEY,
  applyPresetToModel,
  findActiveScopedPresetName,
  normalizeRatioWeights,
  parseModelWeightEntries,
  parseModelWeightPresets,
  presetsForModel,
  type ModelWeightPreset,
} from '@/features/system-settings/models/model-weight-presets'
import { formatPercent } from '@/lib/format'
import { MobileEmpty } from '@/mobile/components/mobile-empty'
import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { ValueRow } from '@/mobile/components/value-row'
import { useChannels } from '@/mobile/features/channels/api'
import { CHANNEL_STATUS_FILTER } from '@/mobile/features/channels/lib/channel-status'
import {
  useRequestPolicy,
  useSaveRequestPolicy,
} from '@/mobile/features/routing-weights/api'
import {
  CustomRatioDialog,
  type CustomRatioTier,
} from '@/mobile/features/routing-weights/components/custom-ratio-dialog'
import {
  applyNormalizedWeights,
  buildModelWeightGroups,
  type ChannelWeightLookup,
  type ModelWeightGroup,
  type ModelWeightRowView,
} from '@/mobile/features/routing-weights/lib/model-weight-groups'
import { ApiError } from '@/mobile/lib/api-client'
import { mobileErrorCopy } from '@/mobile/lib/error-copy'

const CUSTOM_SELECT_VALUE = '__custom__'
const PRESET_SELECT_PREFIX = 'preset:'

function rowKey(channelID: number): string {
  return String(channelID)
}

function channelLabel(row: ModelWeightRowView): string {
  return row.channel?.name ?? `#${row.entry.channel_id}`
}

export function RoutingWeightsPage() {
  const { t } = useTranslation()
  const policy = useRequestPolicy()
  const savePolicy = useSaveRequestPolicy()
  const channels = useChannels({
    name: '',
    statusFilter: CHANNEL_STATUS_FILTER.all,
  })
  const [pendingPreset, setPendingPreset] = useState<{
    model: string
    preset: ModelWeightPreset
  } | null>(null)
  const [customModel, setCustomModel] = useState<string | null>(null)
  const [customValues, setCustomValues] = useState<Record<string, string>>({})
  const [customError, setCustomError] = useState<string | null>(null)

  const weightsRaw = policy.data?.options?.[MODEL_WEIGHTS_OPTION_KEY] ?? ''
  const presetsRaw =
    policy.data?.options?.[MODEL_WEIGHT_PRESETS_OPTION_KEY] ?? ''
  const entries = useMemo(
    () => parseModelWeightEntries(weightsRaw),
    [weightsRaw]
  )
  const presets = useMemo(
    () => parseModelWeightPresets(presetsRaw),
    [presetsRaw]
  )
  // A missing channel is a normal state (deleted channel or a still-loading
  // channels query), so the lookup simply omits it and the row degrades.
  const channelLookup: ChannelWeightLookup = useMemo(() => {
    const items = Array.isArray(channels.data?.items) ? channels.data.items : []
    return new Map(
      items.map((channel) => [
        channel.id,
        {
          name: channel.name,
          priority: channel.priority,
          weight: channel.weight,
        },
      ])
    )
  }, [channels.data])
  const groups = useMemo(
    () => buildModelWeightGroups(entries, channelLookup),
    [entries, channelLookup]
  )

  const customGroup =
    customModel === null
      ? undefined
      : groups.find((group) => group.model === customModel)
  const customTiers = useMemo<CustomRatioTier[]>(() => {
    if (!customGroup) return []
    const tiers: CustomRatioTier[] = []
    let currentPriority: number | null = null
    for (const row of customGroup.rows) {
      const key = rowKey(row.entry.channel_id)
      if (tiers.length === 0 || row.priority !== currentPriority) {
        tiers.push({ priority: row.priority, rows: [] })
        currentPriority = row.priority
      }
      tiers.at(-1)?.rows.push({
        key,
        label: channelLabel(row),
        value: customValues[key] ?? '',
      })
    }
    return tiers
  }, [customGroup, customValues])

  const closeCustom = () => {
    setCustomModel(null)
    setCustomValues({})
    setCustomError(null)
  }

  const reportWriteError = (error: unknown) => {
    if (error instanceof ApiError && error.code === 'forbidden') {
      toast.error(t('Administrator access required'))
      return
    }
    toast.error(t('Save failed'))
  }

  const confirmPreset = async () => {
    if (!pendingPreset) return
    const next = applyPresetToModel(weightsRaw, pendingPreset.preset)
    try {
      await savePolicy.mutateAsync({ [MODEL_WEIGHTS_OPTION_KEY]: next })
      setPendingPreset(null)
      toast.success(t('Saved successfully'))
    } catch (error) {
      reportWriteError(error)
    }
  }

  const openCustom = (group: ModelWeightGroup) => {
    const values: Record<string, string> = {}
    for (const row of group.rows) {
      const key = rowKey(row.entry.channel_id)
      if (row.sharePercent !== null) {
        values[key] = String(row.sharePercent)
      } else if (row.weight !== null) {
        values[key] = String(row.weight)
      } else {
        values[key] = ''
      }
    }
    setCustomModel(group.model)
    setCustomValues(values)
    setCustomError(null)
  }

  const confirmCustom = async () => {
    if (!customGroup) return
    const weightsByChannel = new Map<number, number>()
    let anyPositive = false
    let index = 0
    while (index < customGroup.rows.length) {
      const priority = customGroup.rows[index].priority
      const tier: ModelWeightRowView[] = []
      while (
        index < customGroup.rows.length &&
        customGroup.rows[index].priority === priority
      ) {
        tier.push(customGroup.rows[index])
        index += 1
      }
      const percents = tier.map((row) =>
        Number(customValues[rowKey(row.entry.channel_id)] ?? '')
      )
      if (percents.some((percent) => Number.isFinite(percent) && percent > 0)) {
        anyPositive = true
      }
      const normalized = normalizeRatioWeights(percents)
      tier.forEach((row, tierIndex) => {
        weightsByChannel.set(row.entry.channel_id, normalized[tierIndex])
      })
    }
    if (!anyPositive) {
      setCustomError(t('Enter a ratio greater than 0.'))
      return
    }
    const next = JSON.stringify(
      applyNormalizedWeights(entries, customGroup.model, weightsByChannel)
    )
    try {
      await savePolicy.mutateAsync({ [MODEL_WEIGHTS_OPTION_KEY]: next })
      closeCustom()
      toast.success(t('Saved successfully'))
    } catch (error) {
      reportWriteError(error)
    }
  }

  const handleSelect = (group: ModelWeightGroup, value: string | null) => {
    if (value === null) return
    if (value === CUSTOM_SELECT_VALUE) {
      openCustom(group)
      return
    }
    const name = value.startsWith(PRESET_SELECT_PREFIX)
      ? value.slice(PRESET_SELECT_PREFIX.length)
      : ''
    const preset = presetsForModel(presets, group.model).find(
      (candidate) => candidate.name === name
    )
    if (preset) setPendingPreset({ model: group.model, preset })
  }

  if (policy.isPending) {
    return (
      <div className='px-3 pb-4'>
        <MobileLoading />
      </div>
    )
  }
  if (policy.isError) {
    const forbidden =
      policy.error instanceof ApiError && policy.error.code === 'forbidden'
    const errorCopy = mobileErrorCopy(policy.error)
    return (
      <div className='px-3 pb-4'>
        <MobileError
          title={t(errorCopy.titleKey)}
          description={t(errorCopy.descriptionKey)}
          // Retrying cannot grant a missing role, so only offer it for
          // transient failures.
          onRetry={forbidden ? undefined : () => void policy.refetch()}
        />
      </div>
    )
  }

  return (
    <div className='space-y-4 px-3 pb-4'>
      <h2 className='text-sm font-medium'>{t('Model routing')}</h2>

      {groups.length === 0 ? (
        <MobileEmpty
          title={t('No Data')}
          description={t('Nothing to show for this range.')}
        />
      ) : (
        groups.map((group) => {
          const scopedPresets = presetsForModel(presets, group.model)
          const activeName = findActiveScopedPresetName(
            presets,
            group.model,
            weightsRaw
          )
          const options = [
            ...scopedPresets.map((preset) => ({
              label: preset.name,
              value: `${PRESET_SELECT_PREFIX}${preset.name}`,
            })),
            { label: t('Custom ratio'), value: CUSTOM_SELECT_VALUE },
          ]
          return (
            <section
              key={group.model}
              className='space-y-2 rounded-lg border p-3'
            >
              <div className='flex items-center justify-between gap-2'>
                <h3 className='min-w-0 truncate text-sm font-medium'>
                  {group.model}
                </h3>
                <Select
                  items={options}
                  value={
                    activeName === null
                      ? CUSTOM_SELECT_VALUE
                      : `${PRESET_SELECT_PREFIX}${activeName}`
                  }
                  onValueChange={(value) => handleSelect(group, value)}
                >
                  <SelectTrigger
                    aria-label={`${t('Routing ratio')} ${group.model}`}
                    className='max-w-44'
                  >
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    {options.map((option) => (
                      <SelectItem
                        key={option.value}
                        value={option.value}
                        // A controlled Select does not fire onValueChange when
                        // the user re-picks the already-active Custom option,
                        // so the item itself opens the dialog.
                        onClick={
                          option.value === CUSTOM_SELECT_VALUE
                            ? () => openCustom(group)
                            : undefined
                        }
                      >
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className='divide-y'>
                {group.rows.map((row) => (
                  <ValueRow
                    key={rowKey(row.entry.channel_id)}
                    label={channelLabel(row)}
                    secondary={`#${row.entry.channel_id} · ${t('Weight')} ${
                      row.weight ?? '-'
                    }`}
                    value={
                      row.sharePercent === null
                        ? '-'
                        : formatPercent(row.sharePercent)
                    }
                  />
                ))}
              </div>
            </section>
          )
        })
      )}

      <p className='text-muted-foreground px-1 text-[11px]'>
        {t('The share is computed inside one priority tier.')}
      </p>

      <ConfirmDialog
        open={pendingPreset !== null}
        onOpenChange={(open) => {
          if (!open) setPendingPreset(null)
        }}
        title={t('Apply preset to this model?')}
        desc={t('This replaces the overrides of {{model}} only.', {
          model: pendingPreset?.model ?? '',
        })}
        confirmText={t('Apply')}
        isLoading={savePolicy.isPending}
        handleConfirm={() => void confirmPreset()}
      />

      <CustomRatioDialog
        open={customGroup !== undefined}
        tiers={customTiers}
        error={customError}
        isSaving={savePolicy.isPending}
        onValueChange={(key, value) =>
          setCustomValues((previous) => ({ ...previous, [key]: value }))
        }
        onCancel={closeCustom}
        onConfirm={() => void confirmCustom()}
      />
    </div>
  )
}
