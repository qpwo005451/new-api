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

import { ChevronRight, Weight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

import type {
  ModelMonitorEffectiveModel,
  ModelMonitorSiteResponse,
  ModelMonitorStatus,
} from '../types'

const modelVariant: Record<ModelMonitorStatus, StatusVariant> = {
  available: 'success',
  limited: 'warning',
  unavailable: 'danger',
  unknown: 'neutral',
}

export type ModelSummarySite = {
  site: ModelMonitorSiteResponse
  model: ModelMonitorEffectiveModel
}

type ModelSummaryCardProps = {
  modelName: string
  weight: number
  sites: ModelSummarySite[]
  onOpenSite: (siteId: number, modelName: string) => void
}

function getEffectiveStatus(
  model: ModelMonitorEffectiveModel
): ModelMonitorStatus {
  return model.status === 'unknown' ? model.latest_status : model.status
}

export function ModelSummaryCard(props: ModelSummaryCardProps) {
  const { t } = useTranslation()
  const availableSites = props.sites.filter(
    (item) => getEffectiveStatus(item.model) === 'available'
  ).length

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle className='flex items-center justify-between gap-3'>
          <span className='truncate' title={props.modelName}>
            {props.modelName}
          </span>
          <StatusBadge variant='neutral'>
            <Weight
              data-icon='inline-start'
              className='size-3'
              aria-hidden='true'
            />
            {t('Weight')} {props.weight}
          </StatusBadge>
        </CardTitle>
        <CardDescription>
          {t('{{available}} of {{sites}} sites available', {
            available: availableSites,
            sites: props.sites.length,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='divide-y rounded-lg border'>
          {props.sites.map((item) => {
            const status = getEffectiveStatus(item.model)
            return (
              <button
                key={item.site.site.id}
                type='button'
                className='hover:bg-muted/40 focus-visible:ring-ring/50 flex w-full items-center gap-3 px-3 py-2.5 text-left outline-none first:rounded-t-lg last:rounded-b-lg focus-visible:ring-2 focus-visible:ring-inset'
                onClick={() =>
                  props.onOpenSite(item.site.site.id, props.modelName)
                }
              >
                <span className='min-w-0 flex-1'>
                  <span className='block truncate text-sm font-medium'>
                    {item.site.site.name}
                  </span>
                  <span className='text-muted-foreground mt-0.5 block text-xs'>
                    {t('Weight')} {item.model.weight}
                  </span>
                </span>
                <StatusBadge variant={modelVariant[status]} size='sm'>
                  {t(status)}
                </StatusBadge>
                <ChevronRight
                  className='text-muted-foreground size-4 shrink-0'
                  aria-hidden='true'
                />
              </button>
            )
          })}
        </div>
      </CardContent>
    </Card>
  )
}
