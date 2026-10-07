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
import { Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import {
  findActiveScopedPresetName,
  presetsForModel,
  type ModelWeightPreset,
} from './model-weight-presets'
import {
  CUSTOM_RATIO_VALUE,
  MAX_MODEL_PRIORITY_VALUE,
  MAX_MODEL_WEIGHT_VALUE,
  type ModelWeightChannelOption,
  type ModelWeightRow,
  type RowGroup,
} from './model-weight-rows'

type ModelWeightRowEditorProps = {
  row: ModelWeightRow
  index: number
  share: string
  channelOptions: ModelWeightChannelOption[]
  onRowChange: (index: number, changes: Partial<ModelWeightRow>) => void
  onRemove: (index: number) => void
}

export function ModelWeightRowEditor(props: ModelWeightRowEditorProps) {
  const { t } = useTranslation()

  return (
    <div className='grid gap-3 rounded-lg border p-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_7rem_7rem_5rem_auto]'>
      <label className='grid gap-1.5 text-sm'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Channel')}
        </span>
        <Select
          items={props.channelOptions}
          value={props.row.channel_id > 0 ? String(props.row.channel_id) : null}
          onValueChange={(value) =>
            value !== null &&
            props.onRowChange(props.index, { channel_id: Number(value) })
          }
        >
          <SelectTrigger aria-label={t('Channel')} className='w-full'>
            <SelectValue placeholder={t('Select a channel')} />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            {props.channelOptions.map((option) => (
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
          value={props.row.model}
          onChange={(event) =>
            props.onRowChange(props.index, { model: event.target.value })
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
          value={props.row.weight}
          onChange={(event) =>
            props.onRowChange(props.index, { weight: event.target.value })
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
          value={props.row.priority}
          onChange={(event) =>
            props.onRowChange(props.index, { priority: event.target.value })
          }
        />
      </label>
      <div className='grid gap-1.5 text-sm'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Share')}
        </span>
        <span className='flex h-8 items-center text-sm'>{props.share}</span>
      </div>
      <div className='flex items-end justify-end'>
        <Button
          type='button'
          variant='outline'
          size='icon'
          aria-label={t('Remove')}
          onClick={() => props.onRemove(props.index)}
        >
          <Trash2 />
        </Button>
      </div>
    </div>
  )
}

type ModelWeightGroupProps = {
  group: RowGroup
  shares: Map<string, string> | undefined
  channelOptions: ModelWeightChannelOption[]
  presets: ModelWeightPreset[]
  savedWeights: string
  unsaved: boolean
  onRowChange: (index: number, changes: Partial<ModelWeightRow>) => void
  onRemove: (index: number) => void
  onRatioChange: (model: string, value: string) => void
}

export function ModelWeightGroup(props: ModelWeightGroupProps) {
  const { t } = useTranslation()
  const activeName = findActiveScopedPresetName(
    props.presets,
    props.group.model,
    props.savedWeights
  )
  const ratioItems = [
    ...presetsForModel(props.presets, props.group.model).map((preset) => ({
      label: preset.name,
      value: preset.name,
    })),
    { label: t('Custom ratio'), value: CUSTOM_RATIO_VALUE },
  ]

  return (
    <div className='rounded-lg border'>
      <div className='flex flex-wrap items-center justify-between gap-2 border-b p-3'>
        <div className='flex items-center gap-2'>
          <span className='text-sm font-medium'>{props.group.model}</span>
          {props.unsaved ? (
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
              value !== null && props.onRatioChange(props.group.model, value)
            }
          >
            <SelectTrigger
              aria-label={`${t('Routing ratio')} ${props.group.model}`}
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
        {props.group.rows.map((grouped) => (
          <ModelWeightRowEditor
            key={grouped.row.key}
            row={grouped.row}
            index={grouped.index}
            share={props.shares?.get(grouped.row.key) ?? '-'}
            channelOptions={props.channelOptions}
            onRowChange={props.onRowChange}
            onRemove={props.onRemove}
          />
        ))}
      </div>
    </div>
  )
}
