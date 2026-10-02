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
import { RefreshCw, Route, TriangleAlert } from 'lucide-react'
import { useMemo, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { ErrorState } from '@/components/error-state'
import { SectionPageLayout } from '@/components/layout'
import { LoadingState } from '@/components/loading-state'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { TitledCard } from '@/components/ui/titled-card'
import { formatNumber, formatPercent } from '@/lib/format'

import { getChannelAffinityBindings, getRoutingStats } from './api'
import {
  ChannelDistributionChart,
  LiveAllocationChart,
  SwitchReasonsChart,
  SwitchTrendChart,
} from './components/routing-charts'
import {
  AffinityTable,
  DistributionTable,
  LiveAllocationTable,
  LiveBindingsTable,
  SwitchFlowTable,
} from './components/routing-tables'
import { routingStatsTimeRange, type RoutingStatsWindowPreset } from './lib'
import type { RoutingModelChannelStat } from './types'

const WINDOW_PRESETS: RoutingStatsWindowPreset[] = ['1h', '24h', '7d']

const PRESET_LABELS: Record<RoutingStatsWindowPreset, string> = {
  '1h': 'Last hour',
  '24h': 'Last 24 hours',
  '7d': 'Last 7 days',
}

/** Channel names resolved once per id, for rows that only carry the id. */
function channelNameIndex(
  rows: RoutingModelChannelStat[]
): Map<number, string> {
  const names = new Map<number, string>()
  for (const row of rows) {
    if (row.channel_name) names.set(row.channel_id, row.channel_name)
  }
  return names
}

export function RoutingStatsPage() {
  const { t } = useTranslation()
  const [preset, setPreset] = useState<RoutingStatsWindowPreset>('1h')
  const [range, setRange] = useState(() => routingStatsTimeRange('1h'))
  const [modelDraft, setModelDraft] = useState('')
  const [modelFilter, setModelFilter] = useState('')
  const [refreshNonce, setRefreshNonce] = useState(0)

  const statsQuery = useQuery({
    queryKey: [
      'routing-stats',
      'window',
      range.start_timestamp,
      range.end_timestamp,
      modelFilter,
      refreshNonce,
    ],
    queryFn: () =>
      getRoutingStats({
        start_timestamp: range.start_timestamp,
        end_timestamp: range.end_timestamp,
        model_name: modelFilter === '' ? undefined : modelFilter,
      }),
    refetchOnWindowFocus: false,
  })
  // Live session bindings are current state, so they refresh on their own
  // cadence and are not part of the time window.
  const bindingsQuery = useQuery({
    queryKey: ['routing-stats', 'bindings'],
    queryFn: () => getChannelAffinityBindings(),
    refetchInterval: 30_000,
    refetchOnWindowFocus: false,
  })

  const stats = statsQuery.data?.data
  const bindings = bindingsQuery.data?.data
  const channelNames = useMemo(
    () => channelNameIndex(stats?.by_model_channel ?? []),
    [stats]
  )

  const applyPreset = (next: string | null) => {
    if (next !== '1h' && next !== '24h' && next !== '7d') return
    setPreset(next)
    setRange(routingStatsTimeRange(next))
  }

  // Recomputing the window alone would not refetch when it lands in the same
  // second, and refetching the previous key would pay for a second scan.
  const refresh = () => {
    setRange(routingStatsTimeRange(preset))
    setRefreshNonce((nonce) => nonce + 1)
    void bindingsQuery.refetch()
  }

  const switchRate =
    stats && stats.scanned > 0 ? (stats.switched / stats.scanned) * 100 : 0

  let content: ReactNode
  if (statsQuery.isLoading) {
    content = <LoadingState />
  } else if (statsQuery.isError || !stats) {
    content = (
      <ErrorState
        description={
          statsQuery.data?.message || t('Failed to load routing statistics')
        }
        onRetry={() => void statsQuery.refetch()}
      />
    )
  } else if (stats.requests === 0) {
    content = (
      <EmptyState
        title={t('No routing data in the selected window.')}
        description={t(
          'Run requests, or widen the time window, to collect routing statistics.'
        )}
      />
    )
  } else {
    content = (
      <div className='space-y-4'>
        {stats.truncated && (
          <Alert>
            <TriangleAlert aria-hidden='true' />
            <AlertTitle>{t('Partial switch analysis')}</AlertTitle>
            <AlertDescription>
              {t(
                'The window holds more requests than the {{limit}} row inspection cap, so switch, reason and trend figures cover only the most recent inspected requests. Distribution figures stay exact.',
                { limit: formatNumber(stats.scanned) }
              )}
            </AlertDescription>
          </Alert>
        )}

        <TitledCard
          title={t('Window summary')}
          description={t(
            'Requests in the selected window and the routing outcome behind them.'
          )}
        >
          <div className='grid grid-cols-2 gap-2 p-4 sm:grid-cols-3 sm:p-5 lg:grid-cols-6'>
            <SummaryTile
              label={t('Requests')}
              value={formatNumber(stats.requests)}
            />
            <SummaryTile
              label={t('Errors')}
              value={formatNumber(stats.errors)}
            />
            <SummaryTile
              label={t('Switched requests')}
              value={formatNumber(stats.switched)}
            />
            <SummaryTile
              label={t('Switch rate')}
              value={formatPercent(switchRate)}
            />
            <SummaryTile
              label={t('Sticky requests')}
              value={formatNumber(stats.sticky)}
            />
            <SummaryTile
              label={t('Inspected requests')}
              value={formatNumber(stats.scanned)}
            />
          </div>
        </TitledCard>

        <ChannelDistributionChart rows={stats.by_model_channel} />
        <DistributionTable rows={stats.by_model_channel} />
        <SwitchTrendChart
          points={stats.trend}
          bucketSeconds={stats.window.bucket_seconds}
        />

        <SwitchReasonsChart reasons={stats.switch_reasons} />
        <SwitchFlowTable
          switches={stats.switches}
          channelNames={channelNames}
        />
        <AffinityTable rules={stats.affinity_by_rule} />

        {bindings?.truncated && (
          <Alert>
            <TriangleAlert aria-hidden='true' />
            <AlertDescription>
              {t(
                'More sessions are pinned to a channel than the list below shows.'
              )}
            </AlertDescription>
          </Alert>
        )}

        {bindings && (
          <>
            <LiveAllocationChart entries={bindings.entries} />
            <LiveAllocationTable entries={bindings.entries} />
            <LiveBindingsTable entries={bindings.entries} />
          </>
        )}
      </div>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        <span className='flex items-center gap-2'>
          <Route className='size-5' aria-hidden='true' />
          {t('Routing Statistics')}
        </span>
      </SectionPageLayout.Title>
      <SectionPageLayout.Actions>
        <form
          className='flex items-center gap-2'
          onSubmit={(event) => {
            event.preventDefault()
            setModelFilter(modelDraft.trim())
          }}
        >
          <Input
            value={modelDraft}
            onChange={(event) => setModelDraft(event.target.value)}
            placeholder={t('Filter by model')}
            aria-label={t('Filter by model')}
            className='w-44'
          />
          <Button type='submit' variant='outline'>
            {t('Filter')}
          </Button>
        </form>
        <Select
          items={WINDOW_PRESETS.map((item) => ({
            value: item,
            label: t(PRESET_LABELS[item]),
          }))}
          value={preset}
          onValueChange={applyPreset}
        >
          <SelectTrigger className='w-40' aria-label={t('Time window')}>
            <SelectValue>{t(PRESET_LABELS[preset])}</SelectValue>
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {WINDOW_PRESETS.map((item) => (
                <SelectItem key={item} value={item}>
                  {t(PRESET_LABELS[item])}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Button
          type='button'
          variant='outline'
          size='icon'
          aria-label={t('Refresh')}
          disabled={statsQuery.isFetching}
          onClick={refresh}
        >
          <RefreshCw
            className={statsQuery.isFetching ? 'animate-spin' : undefined}
          />
        </Button>
      </SectionPageLayout.Actions>

      <SectionPageLayout.Content>{content}</SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

function SummaryTile(props: { label: string; value: string }) {
  return (
    <div className='bg-muted/40 rounded-xl px-3 py-2.5'>
      <div className='text-muted-foreground text-[11px] font-medium'>
        {props.label}
      </div>
      <div className='mt-1.5 font-mono text-sm font-semibold tabular-nums'>
        {props.value}
      </div>
    </div>
  )
}
