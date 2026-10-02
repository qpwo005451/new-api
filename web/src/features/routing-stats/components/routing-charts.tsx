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
import { VChart } from '@visactor/react-vchart'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { TitledCard } from '@/components/ui/titled-card'
import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import {
  buildDistributionSpec,
  buildLiveAllocationSpec,
  buildReasonsSpec,
  buildTrendSpec,
} from '../charts'
import { buildTrendSeries } from '../lib'
import type {
  ChannelAffinityBinding,
  RoutingModelChannelStat,
  RoutingReasonStat,
  RoutingTrendPoint,
} from '../types'

function ChartFrame(props: {
  title: string
  description: string
  emptyMessage: string
  spec: Record<string, unknown> | null
}) {
  const { resolvedTheme, themeReady } = useChartTheme()

  return (
    <TitledCard title={props.title} description={props.description}>
      <div className='h-72 w-full'>
        {props.spec == null ? (
          <div className='text-muted-foreground flex h-full items-center justify-center text-sm'>
            {props.emptyMessage}
          </div>
        ) : (
          themeReady && (
            <VChart
              spec={{
                ...props.spec,
                theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                background: 'transparent',
              }}
              option={VCHART_OPTION}
            />
          )
        )}
      </div>
    </TitledCard>
  )
}

/**
 * Shows how traffic actually split across the channels that served each model.
 * The percentage is the share within that model, which is the number worth
 * comparing against the configured weight split.
 */
export function ChannelDistributionChart(props: {
  rows: RoutingModelChannelStat[]
}) {
  const { t } = useTranslation()
  const spec = useMemo(
    () => buildDistributionSpec(props.rows, t),
    [props.rows, t]
  )

  return (
    <ChartFrame
      title={t('Channel distribution')}
      description={t(
        'Observed share of requests per model and channel, compared with the configured weight.'
      )}
      emptyMessage={t('No routing data in the selected window.')}
      spec={spec}
    />
  )
}

export function SwitchTrendChart(props: {
  points: RoutingTrendPoint[]
  bucketSeconds: number
}) {
  const { t } = useTranslation()
  const spec = useMemo(
    () =>
      buildTrendSpec(buildTrendSeries(props.points, props.bucketSeconds), t),
    [props.points, props.bucketSeconds, t]
  )

  return (
    <ChartFrame
      title={t('Switch trend')}
      description={t(
        'Requests and channel switches over the selected window, counted from the inspected decision trail.'
      )}
      emptyMessage={t('No routing data in the selected window.')}
      spec={spec}
    />
  )
}

export function SwitchReasonsChart(props: { reasons: RoutingReasonStat[] }) {
  const { t } = useTranslation()
  const spec = useMemo(
    () => buildReasonsSpec(props.reasons, t),
    [props.reasons, t]
  )

  return (
    <ChartFrame
      title={t('Switch reasons')}
      description={t('Why a request had to leave the channel it started on.')}
      emptyMessage={t('No channel switch in the selected window.')}
      spec={spec}
    />
  )
}

export function LiveAllocationChart(props: {
  entries: ChannelAffinityBinding[]
}) {
  const { t } = useTranslation()
  const spec = useMemo(
    () => buildLiveAllocationSpec(props.entries, t),
    [props.entries, t]
  )

  return (
    <ChartFrame
      title={t('Current session allocation')}
      description={t(
        'How the sessions held right now are spread across channels, per model.'
      )}
      emptyMessage={t('No session is pinned to a channel right now.')}
      spec={spec}
    />
  )
}
