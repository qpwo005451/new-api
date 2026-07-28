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
import {
  Activity,
  AlertTriangle,
  Clock3,
  Gauge,
  History,
  RefreshCw,
  Route,
  Server,
  Weight,
} from 'lucide-react'
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
import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import { formatTimestampRelative, formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { getModelMonitorModel, getModelMonitorSite } from '../api'
import type {
  ModelMonitorEffectiveModel,
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

function getEffectiveStatus(
  model: ModelMonitorEffectiveModel
): ModelMonitorStatus {
  return model.status === 'unknown' ? model.latest_status : model.status
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

function SummaryMetric(props: {
  label: string
  value: React.ReactNode
  icon: React.ReactNode
}) {
  return (
    <div className='flex min-w-0 items-center gap-2.5 px-3 py-2.5 sm:px-4'>
      <span className='text-muted-foreground shrink-0'>{props.icon}</span>
      <div className='min-w-0'>
        <p className='text-muted-foreground truncate text-xs'>{props.label}</p>
        <div className='mt-0.5 truncate text-sm font-semibold tabular-nums'>
          {props.value}
        </div>
      </div>
    </div>
  )
}

function DetailMetric(props: {
  label: string
  value: React.ReactNode
  helper?: React.ReactNode
}) {
  return (
    <div className='min-w-0 px-4 py-3'>
      <p className='text-muted-foreground text-xs'>{props.label}</p>
      <div className='mt-1 text-xl font-semibold tabular-nums'>
        {props.value}
      </div>
      {props.helper && (
        <div className='text-muted-foreground mt-1 truncate text-xs'>
          {props.helper}
        </div>
      )}
    </div>
  )
}

type SiteDetailDialogProps = {
  siteId: number | null
  modelName?: string
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

  let selectedModel: string | undefined
  if (
    siteQuery.data?.summary.models.some(
      (model) => model.model_name === props.modelName
    )
  ) {
    selectedModel = props.modelName
  } else if (
    selection?.siteId === props.siteId &&
    siteQuery.data?.summary.models.some(
      (model) => model.model_name === selection.modelName
    )
  ) {
    selectedModel = selection.modelName
  } else {
    selectedModel = siteQuery.data?.summary.models.toSorted(
      (left, right) =>
        right.weight - left.weight ||
        left.model_name.localeCompare(right.model_name)
    )[0]?.model_name
  }

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
  const selectedSummary = siteQuery.data?.summary.models.find(
    (model) => model.model_name === selectedModel
  )
  const selectedStatus = selectedSummary
    ? getEffectiveStatus(selectedSummary)
    : 'unknown'
  const modelCounts = siteQuery.data?.summary.models.reduce(
    (counts, model) => {
      counts[getEffectiveStatus(model)] += 1
      return counts
    },
    {
      available: 0,
      limited: 0,
      unavailable: 0,
      unknown: 0,
    } satisfies Record<ModelMonitorStatus, number>
  )
  const sortedModels =
    siteQuery.data?.summary.models.toSorted(
      (left, right) =>
        right.weight - left.weight ||
        left.model_name.localeCompare(right.model_name)
    ) ?? []
  const latestAggregates =
    detail?.aggregates
      .toSorted((left, right) => right.hour_start - left.hour_start)
      .slice(0, 12) ?? []

  return (
    <Dialog
      open={props.siteId !== null}
      onOpenChange={(open) => props.onOpenChange(open)}
    >
      <DialogContent className='h-[min(88dvh,52rem)] max-h-[calc(100dvh-1rem)] w-[calc(100vw-1rem)] max-w-6xl gap-0 overflow-hidden p-0 sm:max-h-[calc(100dvh-2rem)] sm:w-[calc(100vw-2rem)] sm:max-w-6xl'>
        {siteQuery.isLoading && (
          <div className='space-y-5 p-5'>
            <div className='space-y-2'>
              <Skeleton className='h-6 w-48' />
              <Skeleton className='h-4 w-72' />
            </div>
            <Skeleton className='h-16 w-full' />
            <div className='grid gap-4 md:grid-cols-[17rem_minmax(0,1fr)]'>
              <Skeleton className='h-96 w-full' />
              <Skeleton className='h-96 w-full' />
            </div>
          </div>
        )}
        {!siteQuery.isLoading && (siteQuery.isError || !siteQuery.data) && (
          <div className='flex h-full flex-col items-center justify-center gap-3 p-6 text-center'>
            <AlertTriangle
              className='text-destructive size-8'
              aria-hidden='true'
            />
            <p className='text-destructive text-sm'>
              {siteQuery.error instanceof Error
                ? siteQuery.error.message
                : t('Failed to load site details')}
            </p>
            <Button
              type='button'
              variant='outline'
              onClick={() => void siteQuery.refetch()}
              disabled={siteQuery.isFetching}
            >
              <RefreshCw
                data-icon='inline-start'
                className={siteQuery.isFetching ? 'animate-spin' : undefined}
                aria-hidden='true'
              />
              {t('Retry')}
            </Button>
          </div>
        )}
        {!siteQuery.isLoading && !siteQuery.isError && siteQuery.data && (
          <div className='flex h-full min-h-0 flex-col'>
            <DialogHeader className='shrink-0 gap-3 border-b px-4 py-4 pr-12 sm:px-5'>
              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                <DialogTitle className='min-w-0 truncate text-lg font-semibold tracking-tight'>
                  {siteQuery.data.site.name}
                </DialogTitle>
                <StatusBadge
                  variant={healthVariant[siteQuery.data.summary.health]}
                >
                  {t(siteQuery.data.summary.health)}
                </StatusBadge>
              </div>
              <DialogDescription className='sr-only'>
                {t('Model details')}
              </DialogDescription>
              <div className='grid grid-cols-2 overflow-hidden rounded-lg border lg:grid-cols-4 lg:[&>*]:border-b-0 lg:[&>*:not(:last-child)]:border-r [&>*:nth-child(-n+2)]:border-b [&>*:nth-child(odd)]:border-r'>
                <SummaryMetric
                  label={t('Score')}
                  value={siteQuery.data.summary.score}
                  icon={<Gauge className='size-4' aria-hidden='true' />}
                />
                <SummaryMetric
                  label={t('Channels')}
                  value={siteQuery.data.channel_ids.length}
                  icon={<Server className='size-4' aria-hidden='true' />}
                />
                <SummaryMetric
                  label={t('Models')}
                  value={siteQuery.data.summary.models.length}
                  icon={<Activity className='size-4' aria-hidden='true' />}
                />
                <SummaryMetric
                  label={t('Last observation')}
                  value={
                    siteQuery.data.latest_observed_at > 0
                      ? formatTimestampRelative(
                          siteQuery.data.latest_observed_at
                        )
                      : '-'
                  }
                  icon={<History className='size-4' aria-hidden='true' />}
                />
              </div>
            </DialogHeader>

            <div className='grid min-h-0 flex-1 md:grid-cols-[17rem_minmax(0,1fr)]'>
              <aside className='flex min-h-0 flex-col border-b md:border-r md:border-b-0'>
                <div className='flex shrink-0 items-center justify-between gap-3 border-b px-4 py-3'>
                  <div>
                    <h3 className='text-sm font-medium'>
                      {t('Monitored models')}
                    </h3>
                    <p className='text-muted-foreground mt-0.5 text-xs'>
                      {t(
                        '{{available}} available · {{issues}} need attention',
                        {
                          available: modelCounts?.available ?? 0,
                          issues:
                            (modelCounts?.limited ?? 0) +
                            (modelCounts?.unavailable ?? 0),
                        }
                      )}
                    </p>
                  </div>
                </div>
                <ScrollArea className='max-h-48 min-h-0 flex-1 md:max-h-none'>
                  <div className='divide-y'>
                    {sortedModels.map((model) => {
                      const effectiveStatus = getEffectiveStatus(model)
                      return (
                        <button
                          key={model.model_name}
                          type='button'
                          className='hover:bg-muted/40 data-[selected=true]:bg-accent/60 focus-visible:ring-ring/50 flex w-full items-center gap-3 px-4 py-2.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-inset'
                          data-selected={selectedModel === model.model_name}
                          onClick={() =>
                            setSelection({
                              siteId: siteQuery.data.site.id,
                              modelName: model.model_name,
                            })
                          }
                        >
                          <span
                            className={cn(
                              'bg-muted size-2 shrink-0 rounded-full',
                              effectiveStatus === 'available' && 'bg-success',
                              effectiveStatus === 'limited' && 'bg-warning',
                              effectiveStatus === 'unavailable' &&
                                'bg-destructive'
                            )}
                            aria-hidden='true'
                          />
                          <span className='min-w-0 flex-1'>
                            <span
                              className='block truncate text-sm font-medium'
                              title={model.model_name}
                            >
                              {model.model_name}
                            </span>
                            <span className='text-muted-foreground mt-0.5 flex items-center gap-1 text-xs'>
                              <Weight className='size-3' aria-hidden='true' />
                              {t('Weight')} {model.weight}
                            </span>
                          </span>
                          <StatusBadge
                            variant={modelVariant[effectiveStatus]}
                            size='sm'
                          >
                            {t(effectiveStatus)}
                          </StatusBadge>
                        </button>
                      )
                    })}
                  </div>
                </ScrollArea>
              </aside>

              <main className='min-h-0 overflow-y-auto'>
                {modelQuery.isLoading && (
                  <div className='space-y-4 p-4 sm:p-5'>
                    <Skeleton className='h-16 w-full' />
                    <Skeleton className='h-24 w-full' />
                    <Skeleton className='h-72 w-full' />
                  </div>
                )}
                {!modelQuery.isLoading && (modelQuery.isError || !detail) && (
                  <div className='flex min-h-72 flex-col items-center justify-center gap-3 p-6 text-center'>
                    <AlertTriangle
                      className='text-destructive size-8'
                      aria-hidden='true'
                    />
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
                  <div className='min-w-0'>
                    <section className='flex flex-col gap-3 border-b px-4 py-4 sm:flex-row sm:items-start sm:justify-between sm:px-5'>
                      <div className='min-w-0'>
                        <div className='flex min-w-0 flex-wrap items-center gap-2'>
                          <h3 className='truncate text-base font-semibold'>
                            {detail.target.model_name}
                          </h3>
                          <StatusBadge variant={modelVariant[selectedStatus]}>
                            {t(selectedStatus)}
                          </StatusBadge>
                        </div>
                        <p className='text-muted-foreground mt-1 text-sm'>
                          {lastException
                            ? lastException.error_summary ||
                              lastException.failure_type
                            : t('No recent exception')}
                        </p>
                      </div>
                      <div className='flex shrink-0 flex-wrap gap-1.5'>
                        <StatusBadge variant='neutral'>
                          <Route
                            data-icon='inline-start'
                            className='size-3'
                            aria-hidden='true'
                          />
                          {t('Endpoint')}: {detail.target.endpoint_type}
                        </StatusBadge>
                        <StatusBadge variant='neutral'>
                          <Weight
                            data-icon='inline-start'
                            className='size-3'
                            aria-hidden='true'
                          />
                          {t('Weight')}: {detail.target.weight}
                        </StatusBadge>
                      </div>
                    </section>

                    <section className='grid border-b sm:grid-cols-3 sm:[&>*:not(:last-child)]:border-r'>
                      <DetailMetric
                        label={t('24h availability')}
                        value={availabilityLabel}
                        helper={t('{{count}} observations', {
                          count: detail.observations.length,
                        })}
                      />
                      <DetailMetric
                        label={t('Latest TTFT')}
                        value={latestTtftLabel}
                        helper={
                          latestObservation
                            ? t('{{duration}} ms total duration', {
                                duration: latestObservation.total_duration_ms,
                              })
                            : undefined
                        }
                      />
                      <DetailMetric
                        label={t('Last observation')}
                        value={
                          latestObservation
                            ? formatTimestampRelative(
                                latestObservation.observed_at
                              )
                            : '-'
                        }
                        helper={
                          latestObservation
                            ? `${t('Source')}: ${t(latestObservation.source)}`
                            : undefined
                        }
                      />
                    </section>

                    <section className='border-b px-4 py-4 sm:px-5'>
                      <div className='mb-3 flex items-center justify-between gap-3'>
                        <div>
                          <h4 className='text-sm font-medium'>
                            {t('Hourly availability trend')}
                          </h4>
                          <p className='text-muted-foreground mt-0.5 text-xs'>
                            {t('Availability (last 24h)')}
                          </p>
                        </div>
                        <Clock3
                          className='text-muted-foreground size-4'
                          aria-hidden='true'
                        />
                      </div>
                      {latestAggregates.length === 0 ? (
                        <p className='text-muted-foreground py-8 text-center text-sm'>
                          {t('No hourly aggregate yet')}
                        </p>
                      ) : (
                        <div className='space-y-2.5'>
                          {latestAggregates.map((item) => {
                            const percentage = Math.round(
                              item.availability_basis_points / 100
                            )
                            return (
                              <div
                                key={item.id}
                                className='grid grid-cols-[minmax(6.5rem,auto)_minmax(0,1fr)_3rem] items-center gap-3 text-xs'
                              >
                                <span className='text-muted-foreground truncate tabular-nums'>
                                  {formatTimestampToDate(item.hour_start)}
                                </span>
                                <div className='bg-muted h-2 overflow-hidden rounded-full'>
                                  <div
                                    className={cn(
                                      'h-full rounded-full bg-success',
                                      percentage < 95 && 'bg-warning',
                                      percentage < 60 && 'bg-destructive'
                                    )}
                                    style={{
                                      width: `${Math.max(2, percentage)}%`,
                                    }}
                                  />
                                </div>
                                <span className='text-right font-medium tabular-nums'>
                                  {percentage}%
                                </span>
                              </div>
                            )
                          })}
                        </div>
                      )}
                    </section>

                    <div className='grid sm:grid-cols-2 sm:[&>*:first-child]:border-r'>
                      <section className='min-w-0 px-4 py-4 sm:px-5'>
                        <div className='mb-3 flex items-center gap-2'>
                          <AlertTriangle
                            className={cn(
                              'text-muted-foreground size-4',
                              lastException && 'text-destructive'
                            )}
                            aria-hidden='true'
                          />
                          <h4 className='text-sm font-medium'>
                            {t('Latest exception')}
                          </h4>
                        </div>
                        {lastException ? (
                          <dl className='space-y-2 text-sm'>
                            <div className='grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3'>
                              <dt className='text-muted-foreground text-xs'>
                                {t('Status')}
                              </dt>
                              <dd>
                                <StatusBadge
                                  variant={modelVariant[lastException.status]}
                                  size='sm'
                                >
                                  {t(lastException.status)}
                                </StatusBadge>
                              </dd>
                            </div>
                            <div className='grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3'>
                              <dt className='text-muted-foreground text-xs'>
                                {t('Exception time')}
                              </dt>
                              <dd className='text-muted-foreground text-xs tabular-nums'>
                                {formatTimestampToDate(
                                  lastException.observed_at
                                )}
                              </dd>
                            </div>
                            <div className='grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3'>
                              <dt className='text-muted-foreground text-xs'>
                                {t('Type')}
                              </dt>
                              <dd className='text-xs break-words'>
                                {lastException.failure_type || '-'}
                              </dd>
                            </div>
                            <div className='grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3'>
                              <dt className='text-muted-foreground text-xs'>
                                {t('Details')}
                              </dt>
                              <dd className='text-muted-foreground text-xs break-words'>
                                {lastException.error_summary || '-'}
                              </dd>
                            </div>
                          </dl>
                        ) : (
                          <p className='text-muted-foreground text-sm'>
                            {t('No recent exception')}
                          </p>
                        )}
                      </section>

                      <section className='min-w-0 border-t px-4 py-4 sm:border-t-0 sm:px-5'>
                        <div className='mb-3 flex items-center gap-2'>
                          <Gauge
                            className='text-muted-foreground size-4'
                            aria-hidden='true'
                          />
                          <h4 className='text-sm font-medium'>
                            {t('Pricing snapshot')}
                          </h4>
                        </div>
                        {detail.pricing ? (
                          <dl className='space-y-2 text-sm'>
                            <div className='grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3'>
                              <dt className='text-muted-foreground text-xs'>
                                {t('Model family')}
                              </dt>
                              <dd className='text-xs break-words'>
                                {detail.pricing.model_family || '-'}
                              </dd>
                            </div>
                            <div className='grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3'>
                              <dt className='text-muted-foreground text-xs'>
                                {t('Billing type')}
                              </dt>
                              <dd className='text-xs break-words'>
                                {detail.pricing.billing_class || '-'}
                              </dd>
                            </div>
                            <div className='grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3'>
                              <dt className='text-muted-foreground text-xs'>
                                {t('Captured at')}
                              </dt>
                              <dd className='text-muted-foreground text-xs tabular-nums'>
                                {formatTimestampToDate(
                                  detail.pricing.captured_at
                                )}
                              </dd>
                            </div>
                          </dl>
                        ) : (
                          <p className='text-muted-foreground text-sm'>
                            {t('No pricing snapshot')}
                          </p>
                        )}
                      </section>
                    </div>
                  </div>
                )}
              </main>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
