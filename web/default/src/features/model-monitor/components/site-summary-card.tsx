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

import { ChevronRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/design-system/button'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatTimestampRelative } from '@/lib/format'

import type {
  ModelMonitorHealth,
  ModelMonitorSiteResponse,
  ModelMonitorStatus,
} from '../types'

const healthVariant: Record<ModelMonitorHealth, StatusVariant> = {
  normal: 'success',
  degraded: 'warning',
  unavailable: 'destructive',
  unknown: 'neutral',
}

const modelVariant: Record<ModelMonitorStatus, StatusVariant> = {
  available: 'success',
  limited: 'warning',
  unavailable: 'destructive',
  unknown: 'neutral',
}

type SiteSummaryCardProps = {
  site: ModelMonitorSiteResponse
  onOpen: () => void
}

export function SiteSummaryCard(props: SiteSummaryCardProps) {
  const { t } = useTranslation()
  const timestamp =
    props.site.freshness_seconds === undefined
      ? undefined
      : Math.floor(Date.now() / 1000) - props.site.freshness_seconds
  const sortedModels = props.site.summary.models.toSorted(
    (left, right) =>
      right.weight - left.weight ||
      left.model_name.localeCompare(right.model_name)
  )

  return (
    <Card size='sm'>
      <CardHeader>
        <CardTitle className='flex items-center justify-between gap-3'>
          <span className='truncate'>{props.site.site.name}</span>
          <StatusBadge variant={healthVariant[props.site.summary.health]}>
            {t(props.site.summary.health)}
          </StatusBadge>
        </CardTitle>
        <CardDescription>
          {t('{{score}} score · {{channels}} channels · {{models}} models', {
            score: props.site.summary.score,
            channels: props.site.channel_ids.length,
            models: props.site.summary.models.length,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        <div className='flex flex-wrap gap-1.5'>
          {sortedModels.map((model) => (
            <StatusBadge
              key={model.model_name}
              variant={
                modelVariant[
                  model.status === 'unknown'
                    ? model.latest_status
                    : model.status
                ]
              }
            >
              {model.model_name}
            </StatusBadge>
          ))}
        </div>
        <div className='flex items-center justify-between gap-3'>
          <p className='text-muted-foreground text-xs'>
            {timestamp === undefined
              ? t('No observations yet')
              : t('Last observation {{time}}', {
                  time: formatTimestampRelative(timestamp),
                })}
          </p>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={props.onOpen}
          >
            {t('Details')}
            <ChevronRight data-icon='inline-end' aria-hidden='true' />
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
