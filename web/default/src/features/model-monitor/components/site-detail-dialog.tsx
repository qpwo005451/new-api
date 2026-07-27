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

import { useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/design-system/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/design-system/dialog'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampRelative, formatTimestampToDate } from '@/lib/format'

import { getModelMonitorModel, getModelMonitorSite } from '../api'
import type {
  ModelMonitorHealth,
  ModelMonitorModelDetail,
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

function getAvailability(detail: ModelMonitorModelDetail): number | undefined {
  const total = detail.aggregates.reduce(
    (sum, item) => sum + item.observation_count,
    0
  )
  if (total === 0) return undefined
  const available = detail.aggregates.reduce(
    (sum, item) => sum + item.available_count,
    0
  )
  return Math.round((available / total) * 100)
}

function getLastException(detail: ModelMonitorModelDetail) {
  return detail.observations.find(
    (item) => item.failure_type || item.error_summary
  )
}

type SiteDetailDialogProps = {
  siteId: number | null
  onOpenChange: (open: boolean) => void
}

export function SiteDetailDialog(props: SiteDetailDialogProps) {
  const { t } = useTranslation()
  const [selection, setSelection] = useState<{
    siteId: number
    modelName: string
  } | null>(null)
  const siteQuery = useQuery({
    queryKey: ['model-monitor', 'site', props.siteId],
    queryFn: async () => {
      const response = await getModelMonitorSite(props.siteId as number)
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load site details'))
      }
      return response.data
    },
    enabled: props.siteId !== null,
    retry: 2,
  })

  const selectedModel =
    selection?.siteId === props.siteId &&
    siteQuery.data?.summary.models.some(
      (model) => model.model_name === selection.modelName
    )
      ? selection.modelName
      : siteQuery.data?.summary.models[0]?.model_name

  const modelQuery = useQuery({
    queryKey: ['model-monitor', 'site', props.siteId, 'model', selectedModel],
    queryFn: async () => {
      const response = await getModelMonitorModel(
        props.siteId as number,
        selectedModel as string
      )
      if (!response.success || !response.data) {
        throw new Error(response.message || t('Failed to load model details'))
      }
      return response.data
    },
    enabled: props.siteId !== null && selectedModel !== undefined,
    retry: 2,
  })

  const detail = modelQuery.data
  const lastException = detail ? getLastException(detail) : undefined
  const availability = detail ? getAvailability(detail) : undefined
  const availabilityLabel =
    availability === undefined ? '-' : `${availability}%`
  const latestTtft = detail?.observations[0]?.first_response_ms
  const latestTtftLabel = latestTtft == null ? '-' : `${latestTtft} ms`
  const latestObservation = detail?.observations[0]

  return (
    <Dialog
      open={props.siteId !== null}
      onOpenChange={(open) => props.onOpenChange(open)}
    >
      <DialogContent className='max-h-[calc(100vh-2rem)] max-w-4xl overflow-y-auto'>
        {siteQuery.isLoading && (
          <div className='space-y-4'>
            <Skeleton className='h-6 w-48' />
            <Skeleton className='h-40 w-full' />
          </div>
        )}
        {!siteQuery.isLoading && (siteQuery.isError || !siteQuery.data) && (
          <div className='text-destructive text-sm'>
            {siteQuery.error instanceof Error
              ? siteQuery.error.message
              : t('Failed to load site details')}
          </div>
        )}
        {!siteQuery.isLoading && !siteQuery.isError && siteQuery.data && (
          <>
            <DialogHeader>
              <DialogTitle className='flex items-center gap-2'>
                <span>{siteQuery.data.site.name}</span>
                <StatusBadge
                  variant={healthVariant[siteQuery.data.summary.health]}
                >
                  {t(siteQuery.data.summary.health)}
                </StatusBadge>
              </DialogTitle>
              <DialogDescription>
                {t('{{channels}} grouped channels · {{score}} score', {
                  channels: siteQuery.data.channel_ids.length,
                  score: siteQuery.data.summary.score,
                })}
              </DialogDescription>
            </DialogHeader>

            <div className='grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]'>
              <section className='space-y-2'>
                <h3 className='text-sm font-medium'>{t('Monitored models')}</h3>
                <div className='space-y-1'>
                  {siteQuery.data.summary.models.map((model) => (
                    <button
                      key={model.model_name}
                      type='button'
                      className='hover:bg-muted/50 data-[selected=true]:bg-muted/50 flex w-full items-center justify-between gap-3 rounded-md border px-3 py-2 text-left text-sm'
                      data-selected={selectedModel === model.model_name}
                      onClick={() =>
                        setSelection({
                          siteId: siteQuery.data.site.id,
                          modelName: model.model_name,
                        })
                      }
                    >
                      <span className='min-w-0 truncate'>
                        {model.model_name}
                      </span>
                      <StatusBadge
                        variant={
                          modelVariant[
                            model.status === 'unknown'
                              ? model.latest_status
                              : model.status
                          ]
                        }
                      >
                        {t(
                          model.status === 'unknown'
                            ? model.latest_status
                            : model.status
                        )}
                      </StatusBadge>
                    </button>
                  ))}
                </div>
              </section>

              <section className='space-y-4'>
                <h3 className='text-sm font-medium'>{t('Model details')}</h3>
                {modelQuery.isLoading && <Skeleton className='h-48 w-full' />}
                {!modelQuery.isLoading && (modelQuery.isError || !detail) && (
                  <div className='space-y-2'>
                    <p className='text-destructive text-sm'>
                      {modelQuery.error instanceof Error
                        ? modelQuery.error.message
                        : t('Failed to load model details')}
                    </p>
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() => void modelQuery.refetch()}
                      disabled={modelQuery.isFetching}
                    >
                      <RefreshCw
                        data-icon='inline-start'
                        className={
                          modelQuery.isFetching ? 'animate-spin' : undefined
                        }
                        aria-hidden='true'
                      />
                      {t('Retry')}
                    </Button>
                  </div>
                )}
                {!modelQuery.isLoading && !modelQuery.isError && detail && (
                  <>
                    <div className='grid gap-2 sm:grid-cols-3'>
                      <div className='rounded-md border p-3'>
                        <p className='text-muted-foreground text-xs'>
                          {t('24h availability')}
                        </p>
                        <p className='text-lg font-semibold tabular-nums'>
                          {availabilityLabel}
                        </p>
                      </div>
                      <div className='rounded-md border p-3'>
                        <p className='text-muted-foreground text-xs'>
                          {t('Latest TTFT')}
                        </p>
                        <p className='text-lg font-semibold tabular-nums'>
                          {latestTtftLabel}
                        </p>
                      </div>
                      <div className='rounded-md border p-3'>
                        <p className='text-muted-foreground text-xs'>
                          {t('Last observation')}
                        </p>
                        <p className='text-sm font-medium'>
                          {latestObservation
                            ? formatTimestampRelative(
                                latestObservation.observed_at
                              )
                            : '-'}
                        </p>
                      </div>
                    </div>

                    <div className='rounded-md border'>
                      <div className='border-b px-3 py-2'>
                        <h4 className='text-sm font-medium'>
                          {t('Hourly availability trend')}
                        </h4>
                      </div>
                      {detail.aggregates.length === 0 ? (
                        <p className='text-muted-foreground p-3 text-sm'>
                          {t('No hourly aggregate yet')}
                        </p>
                      ) : (
                        <div className='max-h-44 overflow-y-auto'>
                          {detail.aggregates.slice(-24).map((item) => (
                            <div
                              key={item.id}
                              className='grid grid-cols-[1fr_auto] gap-3 border-b px-3 py-2 text-sm last:border-b-0'
                            >
                              <span className='text-muted-foreground'>
                                {formatTimestampToDate(item.hour_start)}
                              </span>
                              <span className='tabular-nums'>
                                {Math.round(
                                  item.availability_basis_points / 100
                                )}
                                %
                              </span>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>

                    <div className='grid gap-3 sm:grid-cols-2'>
                      <div className='rounded-md border p-3'>
                        <h4 className='text-sm font-medium'>
                          {t('Latest exception')}
                        </h4>
                        <p className='text-muted-foreground mt-1 text-sm'>
                          {lastException
                            ? lastException.error_summary ||
                              lastException.failure_type
                            : t('No recent exception')}
                        </p>
                      </div>
                      <div className='rounded-md border p-3'>
                        <h4 className='text-sm font-medium'>
                          {t('Pricing snapshot')}
                        </h4>
                        <p className='text-muted-foreground mt-1 text-sm'>
                          {detail.pricing
                            ? t('{{family}} · {{billing}} · {{time}}', {
                                family: detail.pricing.model_family || '-',
                                billing: detail.pricing.billing_class || '-',
                                time: formatTimestampToDate(
                                  detail.pricing.captured_at
                                ),
                              })
                            : t('No pricing snapshot')}
                        </p>
                      </div>
                    </div>
                  </>
                )}
              </section>
            </div>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
