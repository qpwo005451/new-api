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

import { RefreshCw, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { fetchUpstreamModels } from '@/features/channels/api'

import type { ModelMonitorSiteConfig, ModelMonitorTarget } from '../types'

function parseNumbers(value: string): number[] {
  return value
    .split(',')
    .map((item) => Number(item.trim()))
    .filter((item) => Number.isInteger(item) && item > 0)
}

type SiteEditorProps = {
  site: ModelMonitorSiteConfig
  onChange: (site: ModelMonitorSiteConfig) => void
  onRemove: () => void
}

export function SiteEditor(props: SiteEditorProps) {
  const { t } = useTranslation()
  const [isFetchingModels, setIsFetchingModels] = useState(false)
  const [discoveredModels, setDiscoveredModels] = useState<string[]>([])
  const enabledTargets = props.site.targets.filter((target) => target.enabled)
  const selectedModels = enabledTargets.map((target) => target.model_name)
  const modelOptions = useMemo(() => {
    const models = new Set(discoveredModels)
    for (const target of enabledTargets) models.add(target.model_name)
    return [...models]
      .sort((left, right) => left.localeCompare(right))
      .map((model) => ({ label: model, value: model }))
  }, [discoveredModels, enabledTargets])

  useEffect(() => {
    setDiscoveredModels([])
  }, [props.site.id])

  const updateTarget = (
    modelName: string,
    changes: Partial<ModelMonitorTarget>
  ) => {
    props.onChange({
      ...props.site,
      targets: props.site.targets.map((target) =>
        target.model_name === modelName ? { ...target, ...changes } : target
      ),
    })
  }

  const handleModelsChange = (models: string[]) => {
    const currentTargets = new Map(
      props.site.targets.map((target) => [target.model_name, target])
    )
    props.onChange({
      ...props.site,
      targets: models.map((modelName) => {
        const current = currentTargets.get(modelName)
        return current
          ? { ...current, enabled: true }
          : {
              model_name: modelName,
              endpoint_type: 'openai',
              weight: 1,
              enabled: true,
            }
      }),
    })
  }

  const handleFetchModels = async () => {
    if (props.site.channel_ids.length === 0) {
      toast.error(t('Channel IDs'))
      return
    }
    setIsFetchingModels(true)
    try {
      const responses = await Promise.all(
        props.site.channel_ids.map((channelId) =>
          fetchUpstreamModels(channelId)
        )
      )
      const models = new Set<string>()
      for (const response of responses) {
        if (!response.success) {
          throw new Error(response.message || t('Failed to fetch models'))
        }
        for (const model of response.data ?? []) models.add(model)
      }
      setDiscoveredModels([...models])
      toast.success(t('Fetched {{count}} models', { count: models.size }))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to fetch models')
      )
    } finally {
      setIsFetchingModels(false)
    }
  }

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
            onValueChange={(siteType) => {
              if (siteType !== null) {
                props.onChange({
                  ...props.site,
                  site_type: siteType as ModelMonitorSiteConfig['site_type'],
                })
              }
            }}
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
        <div className='grid gap-1.5 text-sm md:col-span-2'>
          <div className='flex items-center justify-between gap-3'>
            <span className='font-medium'>{t('Monitored models')}</span>
            <Button
              type='button'
              variant='outline'
              onClick={() => void handleFetchModels()}
              disabled={isFetchingModels || props.site.channel_ids.length === 0}
            >
              <RefreshCw
                data-icon='inline-start'
                className={isFetchingModels ? 'animate-spin' : undefined}
                aria-hidden='true'
              />
              {t('Fetch Models')}
            </Button>
          </div>
          <MultiSelect
            options={modelOptions}
            selected={selectedModels}
            onChange={handleModelsChange}
            placeholder={t('Monitored models')}
            emptyText={t('Fetch upstream models to load available models')}
            allowCreate
          />
        </div>
        {enabledTargets.length > 0 && (
          <div className='rounded-md border md:col-span-2'>
            <div className='text-muted-foreground grid grid-cols-[minmax(0,1fr)_minmax(8rem,0.45fr)_5rem] gap-3 border-b px-3 py-2 text-xs'>
              <span>{t('Model')}</span>
              <span>{t('Endpoint')}</span>
              <span>{t('Weight')}</span>
            </div>
            {enabledTargets.map((target) => (
              <div
                key={target.model_name}
                className='grid grid-cols-[minmax(0,1fr)_minmax(8rem,0.45fr)_5rem] items-center gap-3 border-b px-3 py-2 last:border-b-0'
              >
                <span className='truncate text-sm'>{target.model_name}</span>
                <Select
                  value={target.endpoint_type || 'openai'}
                  onValueChange={(endpointType) => {
                    if (endpointType === null) return
                    updateTarget(target.model_name, {
                      endpoint_type: endpointType,
                    })
                  }}
                >
                  <SelectTrigger size='sm'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='openai'>OpenAI Chat</SelectItem>
                    <SelectItem value='openai-response'>
                      OpenAI Responses
                    </SelectItem>
                    <SelectItem value='anthropic'>Anthropic</SelectItem>
                    <SelectItem value='gemini'>Gemini</SelectItem>
                  </SelectContent>
                </Select>
                <Select
                  value={String(target.weight)}
                  onValueChange={(weight) => {
                    if (weight === null) return
                    updateTarget(target.model_name, {
                      weight: Number(weight),
                    })
                  }}
                >
                  <SelectTrigger size='sm'>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {[1, 2, 3, 4, 5].map((weight) => (
                      <SelectItem key={weight} value={String(weight)}>
                        {weight}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            ))}
          </div>
        )}
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
