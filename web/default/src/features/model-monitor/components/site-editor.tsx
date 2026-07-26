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
*/

import { Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/design-system/button'
import { Input } from '@/components/design-system/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/design-system/select'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import type { ModelMonitorSiteConfig } from '../types'

function parseNumbers(value: string): number[] {
  return value
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isInteger(item) && item > 0)
}

function parseTargets(value: string) {
  return value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [modelName, endpointType = 'openai', weightValue = '1'] = line
        .split('|')
        .map((item) => item.trim())
      return {
        model_name: modelName,
        endpoint_type: endpointType || 'openai',
        weight: Math.min(5, Math.max(1, Number(weightValue) || 1)),
        enabled: true,
      }
    })
    .filter((target) => target.model_name)
}

function targetsToText(site: ModelMonitorSiteConfig): string {
  return site.targets
    .filter((target) => target.enabled)
    .map(
      (target) =>
        `${target.model_name} | ${target.endpoint_type || 'openai'} | ${target.weight}`
    )
    .join('\n')
}

type SiteEditorProps = {
  site: ModelMonitorSiteConfig
  onChange: (site: ModelMonitorSiteConfig) => void
  onRemove: () => void
}

export function SiteEditor(props: SiteEditorProps) {
  const { t } = useTranslation()
  const [targetText, setTargetText] = useState(() => targetsToText(props.site))

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle>{props.site.name || t('New monitor site')}</CardTitle>
        <CardDescription>
          {t(
            'Channel IDs are grouped as one upstream site. Each model line uses “model | endpoint | weight”.'
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='grid gap-3 md:grid-cols-2'>
        <label className='grid gap-1.5 text-sm'>
          <span className='font-medium'>{t('Site name')}</span>
          <Input
            value={props.site.name}
            onChange={(event) =>
              props.onChange({ ...props.site, name: event.target.value })
            }
          />
        </label>
        <label className='grid gap-1.5 text-sm'>
          <span className='font-medium'>{t('Site type')}</span>
          <Select
            value={props.site.site_type}
            onValueChange={(siteType) =>
              props.onChange({
                ...props.site,
                site_type: siteType as ModelMonitorSiteConfig['site_type'],
              })
            }
          >
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='newapi'>NewAPI</SelectItem>
              <SelectItem value='sub2api'>sub2api</SelectItem>
            </SelectContent>
          </Select>
        </label>
        <label className='grid gap-1.5 text-sm'>
          <span className='font-medium'>{t('Channel IDs')}</span>
          <Input
            value={props.site.channel_ids.join(', ')}
            placeholder='9, 11, 27'
            onChange={(event) =>
              props.onChange({
                ...props.site,
                channel_ids: parseNumbers(event.target.value),
              })
            }
          />
        </label>
        <label className='grid gap-1.5 text-sm'>
          <span className='font-medium'>{t('Pricing group')}</span>
          <Input
            value={props.site.pricing_group ?? ''}
            onChange={(event) =>
              props.onChange({
                ...props.site,
                pricing_group: event.target.value,
              })
            }
          />
        </label>
        <label className='grid gap-1.5 text-sm md:col-span-2'>
          <span className='font-medium'>{t('Monitored models')}</span>
          <Textarea
            rows={Math.max(4, Math.min(10, targetText.split('\n').length + 1))}
            value={targetText}
            placeholder='gpt-5.4 | openai | 1'
            onChange={(event) => {
              setTargetText(event.target.value)
              props.onChange({
                ...props.site,
                targets: parseTargets(event.target.value),
              })
            }}
          />
        </label>
        <div className='flex items-center justify-between gap-3 border-t pt-3 md:col-span-2'>
          <div className='flex items-center gap-2 text-sm'>
            <Switch
              checked={props.site.enabled}
              onCheckedChange={(enabled) =>
                props.onChange({ ...props.site, enabled })
              }
              aria-label={t('Enable site')}
            />
            <span>{t('Enable site')}</span>
          </div>
          <Button type='button' variant='outline' onClick={props.onRemove}>
            <Trash2 data-icon='inline-start' aria-hidden='true' />
            {t('Remove')}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
