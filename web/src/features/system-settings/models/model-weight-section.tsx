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
import { Plus } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { getChannels } from '@/features/channels/api'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSavePolicy } from '../request-policies/use-save-policy'
import { ModelWeightCustomRatioDialog } from './model-weight-custom-ratio-dialog'
import { ModelWeightGroup, ModelWeightRowEditor } from './model-weight-group'
import { ModelWeightPresetBar } from './model-weight-preset-bar'
import {
  applyPresetToModel,
  MODEL_WEIGHTS_OPTION_KEY,
  MODEL_WEIGHT_PRESETS_OPTION_KEY,
  parseModelWeightPresets,
  presetsForModel,
  type ModelWeightPreset,
} from './model-weight-presets'
import {
  applyCustomRatioWeights,
  buildChannelOptions,
  buildCustomRatioState,
  buildDraftRows,
  computeShareByGroupKey,
  computeUnsavedByGroupKey,
  CUSTOM_RATIO_VALUE,
  groupRowsByModel,
  nextModelWeightRowKey,
  parseModelWeights,
  replaceModelRows,
  serializeModelWeights,
  type CustomRatioState,
  type ModelWeightRow,
} from './model-weight-rows'

const MODEL_WEIGHTS_KEY = MODEL_WEIGHTS_OPTION_KEY
const MODEL_WEIGHTS_PRESETS_KEY = MODEL_WEIGHT_PRESETS_OPTION_KEY
const CHANNEL_PAGE_SIZE = 100

type Props = {
  defaultValues: {
    'model_weight_setting.weights': string
    'model_weight_setting.presets': string
  }
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

  const channelOptions = useMemo(
    () => buildChannelOptions(channelsQuery.data, rows),
    [channelsQuery.data, rows]
  )

  const groups = useMemo(
    () => groupRowsByModel(rows, channelById),
    [rows, channelById]
  )

  const draftRows = useMemo(() => buildDraftRows(rows), [rows])

  const shareByGroupKey = useMemo(
    () => computeShareByGroupKey(groups),
    [groups]
  )

  const unsavedByGroupKey = useMemo(
    () => computeUnsavedByGroupKey(groups, rows, savedWeights),
    [groups, rows, savedWeights]
  )

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

  const handleCustomPercentChange = (key: string, value: string) => {
    setCustomRatio((prev) =>
      prev ? { ...prev, percents: { ...prev.percents, [key]: value } } : prev
    )
  }

  const handleCustomConfirm = async () => {
    if (!customRatio) return
    const merged = applyCustomRatioWeights(savedWeights, customRatio)
    const { model } = customRatio
    await savePolicy.mutateAsync({ [MODEL_WEIGHTS_KEY]: merged })
    baselineRef.current = merged
    setSavedWeights(merged)
    setRows((prev) => replaceModelRows(prev, model, merged))
    setCustomRatio(null)
  }

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
            {groups.map((group) => (
              <ModelWeightGroup
                key={group.key}
                group={group}
                shares={shareByGroupKey.get(group.key)}
                channelOptions={channelOptions}
                presets={presets}
                savedWeights={savedWeights}
                unsaved={unsavedByGroupKey.get(group.key) ?? false}
                onRowChange={updateRow}
                onRemove={handleRemove}
                onRatioChange={handleRatioChange}
              />
            ))}
            {draftRows.length > 0 ? (
              <div className='space-y-3 rounded-lg border border-dashed p-3'>
                {draftRows.map(({ row, index }) => (
                  <ModelWeightRowEditor
                    key={row.key}
                    row={row}
                    index={index}
                    share='-'
                    channelOptions={channelOptions}
                    onRowChange={updateRow}
                    onRemove={handleRemove}
                  />
                ))}
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

      <ModelWeightCustomRatioDialog
        state={customRatio}
        isPending={savePolicy.isPending}
        onOpenChange={(open) => {
          if (!open) setCustomRatio(null)
        }}
        onPercentChange={handleCustomPercentChange}
        onConfirm={() => void handleCustomConfirm()}
      />
    </SettingsSection>
  )
}
