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
import { Fragment, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from '@/components/ui/input-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'

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

/**
 * Allocation rows share one grid so the channel name, the advanced override
 * inputs and the share control line up in the same columns on every row.
 * The name column hugs the longest channel name (capped, then truncated) and
 * the trailing `1fr` keeps the tier subheader bands and hover highlights
 * spanning the full card.
 */
const COLLAPSED_GRID =
  'grid grid-cols-[fit-content(50%)_minmax(0,1fr)] sm:grid-cols-[fit-content(14rem)_minmax(0,1fr)]'
const ADVANCED_GRID =
  'flex flex-col sm:grid sm:grid-cols-[fit-content(14rem)_5.5rem_5.5rem_minmax(0,1fr)]'

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
          <span className='truncate text-sm font-semibold'>{props.card.model}</span>
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

      {props.card.tiers.length === 0 ? (
        <p className='text-muted-foreground px-3 py-3 text-sm'>
          {t('No enabled channel serves this model.')}
        </p>
      ) : (
        <div
          className={cn(
            'divide-y gap-x-3',
            advanced ? ADVANCED_GRID : COLLAPSED_GRID
          )}
        >
          {advanced ? (
            <div className='text-muted-foreground col-span-full flex flex-wrap items-center gap-x-3 pb-1 text-xs sm:grid sm:grid-cols-subgrid'>
              <span className='h-5 w-full sm:h-auto sm:w-auto' aria-hidden='true' />
              <span className='w-[5.5rem] shrink-0 sm:w-auto'>
                {t('Priority')}
              </span>
              <span className='w-[5.5rem] shrink-0 sm:w-auto'>
                {t('Weight')}
              </span>
              <span className='w-24 shrink-0 sm:w-auto'>{t('Share')}</span>
            </div>
          ) : null}
          {props.card.tiers.map((tier) => (
            <Fragment key={tier.priority}>
              <div className='bg-muted/40 col-span-full flex items-center gap-2 px-3 py-1'>
                <span className='text-muted-foreground text-xs font-medium'>
                  {`${t('Priority')} ${tier.priority}`}
                </span>
                <Badge
                  variant='outline'
                  className={cn(
                    'h-4 rounded px-1 text-[0.65rem] font-normal',
                    tier.participates
                      ? 'border-primary/30 bg-primary/10 text-primary'
                      : 'border-transparent text-muted-foreground/70'
                  )}
                >
                  {tier.participates ? t('Participating') : t('Fallback')}
                </Badge>
                {tier.totalPercent === 100 ? null : (
                  <span className='text-destructive ml-auto text-xs font-medium tabular-nums'>
                    {`${t('Total')} ${tier.totalPercent}%`}
                  </span>
                )}
              </div>
              {tier.rows.map((row) => (
                <div
                  key={row.key}
                  className={cn(
                    'col-span-full items-center gap-x-3 py-1 transition-colors hover:bg-muted/40',
                    advanced
                      ? 'flex flex-wrap gap-y-1 sm:grid sm:grid-cols-subgrid'
                      : 'grid grid-cols-subgrid'
                  )}
                >
                  <span
                    title={row.channelName}
                    className={cn(
                      'min-w-0 truncate ps-3 text-sm',
                      advanced && 'w-full sm:w-auto'
                    )}
                  >
                    {row.channelName}
                  </span>
                  {advanced ? (
                    <>
                      <Input
                        aria-label={`${t('Priority')} ${props.card.model} #${row.channelId}`}
                        type='number'
                        min={0}
                        max={MAX_MODEL_PRIORITY_VALUE}
                        placeholder={String(row.channelPriority)}
                        value={row.priority}
                        className='h-8 w-[5.5rem] shrink-0 text-right tabular-nums sm:w-full'
                        onChange={(event) =>
                          props.onPriorityChange(row.key, event.target.value)
                        }
                      />
                      <Input
                        aria-label={`${t('Weight')} ${props.card.model} #${row.channelId}`}
                        type='number'
                        min={0}
                        max={MAX_MODEL_WEIGHT_VALUE}
                        value={String(derivedWeightFromPercent(row.percent))}
                        className='h-8 w-[5.5rem] shrink-0 text-right tabular-nums sm:w-full'
                        onChange={(event) =>
                          props.onWeightChange(row.key, event.target.value)
                        }
                      />
                    </>
                  ) : null}
                  <div className='flex min-w-0 items-center gap-2'>
                    <div
                      className='bg-muted hidden h-1 min-w-0 flex-1 overflow-hidden rounded-full sm:block'
                      aria-hidden='true'
                    >
                      <div
                        className='bg-primary/50 h-full rounded-full'
                        style={{
                          width: `${Math.min(100, Math.max(0, Number(row.percent) || 0))}%`,
                        }}
                      />
                    </div>
                    <InputGroup className='h-8 w-24 shrink-0 sm:w-[5.5rem]'>
                      <InputGroupInput
                        aria-label={`${t('Share')} ${props.card.model} #${row.channelId}`}
                        type='number'
                        min={0}
                        max={100}
                        step='any'
                        className='h-8 text-right tabular-nums'
                        value={row.percent}
                        onChange={(event) =>
                          props.onPercentChange(row.key, event.target.value)
                        }
                      />
                      <InputGroupAddon align='inline-end' className='pl-0'>
                        %
                      </InputGroupAddon>
                    </InputGroup>
                  </div>
                </div>
              ))}
            </Fragment>
          ))}
        </div>
      )}
    </div>
  )
}
