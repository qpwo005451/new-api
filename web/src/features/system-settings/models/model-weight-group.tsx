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
import { ChevronDown, ChevronRight } from 'lucide-react'
import { useState } from 'react'
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
  MAX_MODEL_PRIORITY_VALUE,
  MAX_MODEL_WEIGHT_VALUE,
  derivedWeightFromPercent,
  type ModelAllocationCard,
} from './model-weight-rows'

const CUSTOM_PRESET_VALUE = '__custom_preset__'

type Props = {
  card: ModelAllocationCard
  presets: ModelWeightPreset[]
  savedWeights: string
  unsaved: boolean
  onPercentChange: (key: string, value: string) => void
  onPriorityChange: (key: string, value: string) => void
  onWeightChange: (key: string, value: string) => void
  onApplyPreset: (model: string, preset: ModelWeightPreset) => void
  onAverageSplit: (model: string) => void
}

export function ModelWeightCard(props: Props) {
  const { t } = useTranslation()
  const [advanced, setAdvanced] = useState(false)

  const scopedPresets = presetsForModel(props.presets, props.card.model)
  const activeName = findActiveScopedPresetName(
    props.presets,
    props.card.model,
    props.savedWeights
  )
  const presetItems = [
    ...scopedPresets.map((preset) => ({
      label: preset.name,
      value: preset.name,
    })),
    { label: t('Custom'), value: CUSTOM_PRESET_VALUE },
  ]

  const handlePresetChange = (value: string | null) => {
    if (value === null || value === CUSTOM_PRESET_VALUE) return
    const preset = scopedPresets.find((candidate) => candidate.name === value)
    if (preset) props.onApplyPreset(props.card.model, preset)
  }

  return (
    <div className='rounded-lg border'>
      <div className='flex flex-wrap items-center justify-between gap-2 border-b p-3'>
        <div className='flex min-w-0 items-center gap-2'>
          <span className='truncate text-sm font-medium'>{props.card.model}</span>
          <span className='text-muted-foreground text-xs whitespace-nowrap'>
            {t('{{count}} channels', { count: props.card.channelCount })}
          </span>
          {props.unsaved ? (
            <span className='bg-muted text-muted-foreground rounded px-1.5 py-0.5 text-[0.7rem] font-medium'>
              {t('Unsaved changes')}
            </span>
          ) : null}
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          <Select
            items={presetItems}
            value={activeName ?? CUSTOM_PRESET_VALUE}
            onValueChange={handlePresetChange}
          >
            <SelectTrigger
              aria-label={`${t('Preset')} ${props.card.model}`}
              className='w-40'
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              {presetItems.map((item) => (
                <SelectItem key={item.value} value={item.value}>
                  {item.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            type='button'
            size='sm'
            variant='outline'
            onClick={() => props.onAverageSplit(props.card.model)}
          >
            {t('Average split')}
          </Button>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            aria-expanded={advanced}
            onClick={() => setAdvanced((previous) => !previous)}
          >
            {advanced ? <ChevronDown /> : <ChevronRight />}
            {t('Advanced')}
          </Button>
        </div>
      </div>

      <div className='space-y-3 p-3'>
        {props.card.tiers.length === 0 ? (
          <p className='text-muted-foreground text-sm'>
            {t('No enabled channel serves this model.')}
          </p>
        ) : (
          props.card.tiers.map((tier) => (
            <div key={tier.priority} className='space-y-2'>
              {props.card.tiers.length > 1 ? (
                <div className='flex items-center justify-between gap-2 text-xs'>
                  <span className='text-muted-foreground font-medium'>
                    {`${t('Priority')} ${tier.priority}`}
                  </span>
                  <span className='text-muted-foreground'>
                    {tier.participates ? t('Participating') : t('Fallback')}
                  </span>
                </div>
              ) : null}
              {tier.rows.map((row) => (
                <div
                  key={row.key}
                  className='grid items-center gap-2 sm:grid-cols-[minmax(0,1fr)_6rem]'
                >
                  <span className='truncate text-sm'>{row.channelName}</span>
                  <div className='flex items-center gap-1'>
                    <Input
                      aria-label={`${t('Share')} ${props.card.model} #${row.channelId}`}
                      type='number'
                      min={0}
                      max={100}
                      step='any'
                      className='text-right'
                      value={row.percent}
                      onChange={(event) =>
                        props.onPercentChange(row.key, event.target.value)
                      }
                    />
                    <span className='text-muted-foreground text-xs'>%</span>
                  </div>
                  {advanced ? (
                    <div className='grid gap-2 sm:col-span-2 sm:grid-cols-2'>
                      <label className='grid gap-1 text-xs'>
                        <span className='text-muted-foreground font-medium'>
                          {t('Priority')}
                        </span>
                        <Input
                          aria-label={`${t('Priority')} ${props.card.model} #${row.channelId}`}
                          type='number'
                          min={0}
                          max={MAX_MODEL_PRIORITY_VALUE}
                          placeholder={String(row.channelPriority)}
                          value={row.priority}
                          onChange={(event) =>
                            props.onPriorityChange(
                              row.key,
                              event.target.value
                            )
                          }
                        />
                      </label>
                      <label className='grid gap-1 text-xs'>
                        <span className='text-muted-foreground font-medium'>
                          {t('Weight')}
                        </span>
                        <Input
                          aria-label={`${t('Weight')} ${props.card.model} #${row.channelId}`}
                          type='number'
                          min={0}
                          max={MAX_MODEL_WEIGHT_VALUE}
                          value={String(
                            derivedWeightFromPercent(row.percent)
                          )}
                          onChange={(event) =>
                            props.onWeightChange(row.key, event.target.value)
                          }
                        />
                      </label>
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          ))
        )}
      </div>
    </div>
  )
}
