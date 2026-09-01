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
import { VChart } from '@visactor/react-vchart'
import { Database, LineChart } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { IconBadge } from '@/components/ui/icon-badge'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { getTokenTrend, getTokenTrendModels } from '@/features/dashboard/api'
import { computeTimeRange } from '@/lib/time'
import { VCHART_OPTION } from '@/lib/vchart'

import {
  buildTokenTrendSeries,
  buildTokenTrendSpec,
  formatCompactTokens,
} from '../../lib/token-trend'

const TOKEN_TREND_WINDOW_DAYS = 7

export function TokenTrendPanel() {
  const { t } = useTranslation()
  const timeRange = useMemo(() => computeTimeRange(TOKEN_TREND_WINDOW_DAYS), [])
  const [selectedModel, setSelectedModel] = useState<string>('all')

  const modelsQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'token-trend-models',
      timeRange.start_timestamp,
      timeRange.end_timestamp,
    ],
    queryFn: () =>
      getTokenTrendModels({
        start_timestamp: timeRange.start_timestamp,
        end_timestamp: timeRange.end_timestamp,
      }),
    staleTime: 60 * 1000,
    select: (data) => data.data ?? [],
  })

  const trendQuery = useQuery({
    queryKey: [
      'dashboard',
      'overview',
      'token-trend',
      timeRange.start_timestamp,
      timeRange.end_timestamp,
      selectedModel,
    ],
    queryFn: () =>
      getTokenTrend({
        start_timestamp: timeRange.start_timestamp,
        end_timestamp: timeRange.end_timestamp,
        model_name: selectedModel === 'all' ? undefined : selectedModel,
      }),
    staleTime: 60 * 1000,
    select: (data) => data.data ?? [],
    placeholderData: (previous) => previous,
  })

  const series = useMemo(
    () => buildTokenTrendSeries(trendQuery.data ?? []),
    [trendQuery.data]
  )

  const totals = useMemo(
    () => ({
      input: series.reduce((sum, row) => sum + row.input, 0),
      output: series.reduce((sum, row) => sum + row.output, 0),
      cacheRead: series.reduce((sum, row) => sum + row.cacheRead, 0),
      cacheWrite: series.reduce((sum, row) => sum + row.cacheWrite, 0),
      cachePrompt: series.reduce((sum, row) => sum + row.cachePrompt, 0),
    }),
    [series]
  )
  // The backend excludes non-cache-reporting channels (Ollama) from
  // cachePrompt, so it is the honest denominator for the hit rate.
  const hitRate =
    totals.cachePrompt > 0 ? (totals.cacheRead / totals.cachePrompt) * 100 : 0

  const loading = trendQuery.isLoading
  const hasData = series.length > 0

  let chartBody: React.ReactNode
  if (loading) {
    chartBody = <Skeleton className='h-64 w-full rounded-xl' />
  } else if (hasData) {
    chartBody = (
      <div className='relative'>
        <div className='h-64'>
          <VChart
            spec={buildTokenTrendSpec(series, t)}
            option={VCHART_OPTION}
          />
        </div>
        <div className='bg-muted/40 absolute top-2 right-2 flex items-center gap-1.5 rounded-lg px-2.5 py-1.5'>
          <Database
            className='text-muted-foreground size-3.5'
            aria-hidden='true'
          />
          <span className='text-muted-foreground text-[11px]'>
            {t('Cache hit rate')}
          </span>
          <span className='font-mono text-xs font-semibold tabular-nums'>
            {hitRate.toFixed(1)}%
          </span>
        </div>
      </div>
    )
  } else {
    chartBody = (
      <div className='text-muted-foreground flex h-64 items-center justify-center text-sm'>
        {t('No token usage in this period')}
      </div>
    )
  }

  return (
    <section className='bg-card h-full overflow-hidden rounded-2xl border shadow-xs'>
      <div className='flex flex-wrap items-center gap-2 border-b px-4 py-3 sm:px-5'>
        <IconBadge tone='chart-4' size='sm'>
          <LineChart />
        </IconBadge>
        <h3 className='text-sm font-semibold'>{t('Token usage trend')}</h3>
        <span className='text-muted-foreground ml-auto hidden text-xs lg:inline'>
          {t('Cache hit rate for the last 7 days')}
        </span>
        <Select
          value={selectedModel}
          onValueChange={(value) => setSelectedModel(value ?? 'all')}
        >
          <SelectTrigger
            size='sm'
            className='w-44 shrink-0'
            aria-label={t('Model')}
          >
            <SelectValue placeholder={t('All Models')} />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='all'>{t('All Models')}</SelectItem>
              {(modelsQuery.data ?? []).map((model) => (
                <SelectItem key={model} value={model}>
                  {model}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>

      <div className='space-y-3 p-4 sm:p-5'>
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
          <TrendStat
            label={t('Input')}
            value={formatCompactTokens(totals.input)}
            loading={loading}
          />
          <TrendStat
            label={t('Output')}
            value={formatCompactTokens(totals.output)}
            loading={loading}
          />
          <TrendStat
            label={t('Cache Read')}
            value={formatCompactTokens(totals.cacheRead)}
            loading={loading}
          />
          <TrendStat
            label={t('Cache Write')}
            value={formatCompactTokens(totals.cacheWrite)}
            loading={loading}
          />
        </div>

        {chartBody}
      </div>
    </section>
  )
}

function TrendStat(props: { label: string; value: string; loading: boolean }) {
  return (
    <div className='bg-muted/40 rounded-xl px-3 py-2.5'>
      <div className='text-muted-foreground text-[11px] font-medium'>
        {props.label}
      </div>
      {props.loading ? (
        <Skeleton className='mt-1.5 h-5 w-16' />
      ) : (
        <div className='mt-1.5 font-mono text-sm font-semibold tabular-nums'>
          {props.value}
        </div>
      )}
    </div>
  )
}
