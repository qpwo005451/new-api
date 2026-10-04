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

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSavePolicy } from '../request-policies/use-save-policy'
import { ModelWeightPresetBar } from './model-weight-preset-bar'
import {
  MODEL_WEIGHTS_OPTION_KEY,
  parseModelWeightPresets,
} from './model-weight-presets'

const MODEL_WEIGHTS_KEY = MODEL_WEIGHTS_OPTION_KEY
const MODEL_WEIGHT_PRESETS_KEY = 'model_weight_setting.presets'
const CHANNEL_PAGE_SIZE = 100
const MAX_MODEL_WEIGHT_VALUE = 1000000
const MAX_MODEL_PRIORITY_VALUE = 1000000000

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

let modelWeightRowKeySeq = 0

const nextModelWeightRowKey = () => `model-weight-${++modelWeightRowKeySeq}`

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

export function ModelWeightSection(props: Props) {
  const { t } = useTranslation()
  const savePolicy = useSavePolicy()
  const initialRows = useMemo(
    () => parseModelWeights(props.defaultValues[MODEL_WEIGHTS_KEY]),
    [props.defaultValues]
  )
  const [rows, setRows] = useState<ModelWeightRow[]>(initialRows)
  const baselineRef = useRef(serializeModelWeights(initialRows))

  useEffect(() => {
    const parsed = parseModelWeights(props.defaultValues[MODEL_WEIGHTS_KEY])
    setRows(parsed)
    baselineRef.current = serializeModelWeights(parsed)
  }, [props.defaultValues])

  const channelsQuery = useQuery({
    queryKey: ['system-settings', 'model-weights', 'channels'],
    queryFn: fetchAllChannels,
    retry: false,
  })

  const presets = useMemo(
    () =>
      parseModelWeightPresets(props.defaultValues[MODEL_WEIGHT_PRESETS_KEY]),
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
          currentWeights={serializeModelWeights(rows)}
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
          <div className='space-y-3'>
            {rows.map((row, index) => (
              <div
                key={row.key}
                className='grid gap-3 rounded-lg border p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_7rem_7rem_auto]'
              >
                <label className='grid gap-1.5 text-sm'>
                  <span className='text-muted-foreground text-xs font-medium'>
                    {t('Channel')}
                  </span>
                  <Select
                    items={channelOptions}
                    value={row.channel_id > 0 ? String(row.channel_id) : null}
                    onValueChange={(value) =>
                      value !== null &&
                      updateRow(index, { channel_id: Number(value) })
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
                    onChange={(event) =>
                      updateRow(index, { model: event.target.value })
                    }
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
                    onChange={(event) =>
                      updateRow(index, { weight: event.target.value })
                    }
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
            ))}
          </div>
        )}
      </SettingsForm>
    </SettingsSection>
  )
}
