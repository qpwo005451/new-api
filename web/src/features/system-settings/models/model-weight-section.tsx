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
import { Info } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { getChannels } from '@/features/channels/api'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSavePolicy } from '../request-policies/use-save-policy'
import { ModelWeightCard } from './model-weight-group'
import { ModelWeightPresetBar } from './model-weight-preset-bar'
import {
  MODEL_WEIGHTS_OPTION_KEY,
  MODEL_WEIGHT_PRESETS_OPTION_KEY,
  parseModelWeightEntries,
  parseModelWeightPresets,
  type ModelWeightPreset,
} from './model-weight-presets'
import {
  applyScopedPresetToRows,
  averageSplitRows,
  buildAllocationRows,
  computeDirtyModelKeys,
  groupAllocationCards,
  modelKey,
  serializeAllocationDraft,
  setRowPercent,
  setRowPriority,
  type ModelAllocationRow,
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
  const defaultWeights = props.defaultValues[MODEL_WEIGHTS_KEY]
  const presetsRaw = props.defaultValues[MODEL_WEIGHTS_PRESETS_KEY]

  const [savedEntries, setSavedEntries] = useState(() =>
    parseModelWeightEntries(defaultWeights)
  )
  const [baseEntries, setBaseEntries] = useState(() =>
    parseModelWeightEntries(defaultWeights)
  )
  const [rows, setRows] = useState<ModelAllocationRow[]>([])

  useEffect(() => {
    const parsed = parseModelWeightEntries(defaultWeights)
    setSavedEntries(parsed)
    setBaseEntries(parsed)
  }, [defaultWeights])

  const channelsQuery = useQuery({
    queryKey: ['system-settings', 'model-weights', 'channels'],
    queryFn: fetchAllChannels,
    retry: false,
  })
  const channels = channelsQuery.data

  const channelsSignature = useMemo(
    () =>
      channels
        ? JSON.stringify(
            channels.map((channel) => [
              channel.id,
              channel.status,
              channel.priority,
              channel.weight,
              channel.models,
              channel.model_mapping,
            ])
          )
        : '',
    [channels]
  )

  const appliedSignatureRef = useRef('')
  useEffect(() => {
    const signature = `${JSON.stringify(baseEntries)}||${channelsSignature}`
    if (appliedSignatureRef.current === signature) return
    appliedSignatureRef.current = signature
    setRows(buildAllocationRows(baseEntries, channels ?? []))
  }, [baseEntries, channels, channelsSignature])

  const baselineRows = useMemo(
    () => buildAllocationRows(savedEntries, channels ?? []),
    [savedEntries, channels]
  )

  const dirtyModelKeys = useMemo(
    () => computeDirtyModelKeys(rows, baselineRows),
    [rows, baselineRows]
  )

  const cardModels = useMemo(() => {
    const models: string[] = []
    const seen = new Set<string>()
    for (const entry of baseEntries) {
      const model = entry.model.trim()
      const key = modelKey(model)
      if (model === '' || seen.has(key)) continue
      seen.add(key)
      models.push(model)
    }
    return models
  }, [baseEntries])

  const cards = useMemo(
    () => groupAllocationCards(rows, cardModels),
    [rows, cardModels]
  )

  const presets = useMemo(
    () => parseModelWeightPresets(presetsRaw),
    [presetsRaw]
  )

  const editorWeights = useMemo(
    () => serializeAllocationDraft(baseEntries, rows, dirtyModelKeys),
    [baseEntries, rows, dirtyModelKeys]
  )

  const savedWeights = useMemo(() => JSON.stringify(savedEntries), [savedEntries])

  const handleSave = async () => {
    if (dirtyModelKeys.size === 0) {
      toast.info(t('No changes to save'))
      return
    }
    const serialized = serializeAllocationDraft(
      baseEntries,
      rows,
      dirtyModelKeys
    )
    await savePolicy.mutateAsync({ [MODEL_WEIGHTS_KEY]: serialized })
    const merged = parseModelWeightEntries(serialized)
    setSavedEntries(merged)
    setBaseEntries(merged)
  }

  const handlePercentChange = (key: string, value: string) => {
    setRows((previous) => setRowPercent(previous, key, value))
  }

  const handleWeightChange = (key: string, value: string) => {
    setRows((previous) => setRowPercent(previous, key, value))
  }

  const handlePriorityChange = (key: string, value: string) => {
    setRows((previous) => setRowPriority(previous, key, value))
  }

  const handleAverageSplit = (model: string) => {
    setRows((previous) => averageSplitRows(previous, model))
  }

  const handleApplyScopedPreset = (
    model: string,
    preset: ModelWeightPreset
  ) => {
    setRows((previous) => applyScopedPresetToRows(previous, model, preset))
  }

  const handleApplyGlobalPreset = (preset: ModelWeightPreset) => {
    setBaseEntries(preset.weights.map((entry) => ({ ...entry })))
  }

  return (
    <SettingsSection title={t('Model Routing')}>
      <div className='flex items-center gap-1.5'>
        <p className='text-muted-foreground text-sm font-medium'>
          {t('Per-model channel routing')}
        </p>
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='size-5'
                  aria-label={t('Help')}
                >
                  <Info className='size-3.5' aria-hidden='true' />
                </Button>
              }
            />
            <TooltipContent className='max-w-sm'>
              {t('Overrides the channel priority or weight for one model only.')}{' '}
              {t('Unset fields keep the channel value.')}{' '}
              {t('A weight of 0 excludes the model on this channel.')}{' '}
              {t(
                'Priority decides which channels compete; weight splits traffic within one priority.'
              )}{' '}
              {t('The share is computed inside one priority tier.')}
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>

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
          editorWeights={editorWeights}
          onApplyPreset={handleApplyGlobalPreset}
        />

        {cards.length === 0 ? (
          <div className='text-muted-foreground/80 rounded-lg border border-dashed px-5 py-8 text-center text-sm'>
            {t('No model overrides yet.')}
          </div>
        ) : (
          <div className='space-y-4'>
            {cards.map((card) => (
              <ModelWeightCard
                key={card.key}
                card={card}
                presets={presets}
                savedWeights={savedWeights}
                unsaved={dirtyModelKeys.has(card.key)}
                onPercentChange={handlePercentChange}
                onPriorityChange={handlePriorityChange}
                onWeightChange={handleWeightChange}
                onApplyPreset={handleApplyScopedPreset}
                onAverageSplit={handleAverageSplit}
              />
            ))}
          </div>
        )}
      </SettingsForm>
    </SettingsSection>
  )
}
