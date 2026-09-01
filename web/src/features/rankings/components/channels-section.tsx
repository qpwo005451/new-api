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
import { Server } from 'lucide-react'
import { useCallback, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { useChartTheme } from '@/lib/use-chart-theme'
import { VCHART_OPTION } from '@/lib/vchart'

import { formatShare, formatTokens } from '../lib/format'
import type { ChannelShareSeries, RankedChannel, RankingPeriod } from '../types'
import { GrowthText } from './growth-text'

const PERIOD_DESCRIPTIONS: Record<RankingPeriod, string> = {
  today: 'Token share by channel across the last 24 hours',
  week: 'Token share by channel across the past few weeks',
  month: 'Token share by channel across the past month',
  year: 'Token share by channel across the past year',
}

/** Fixed colour for the "Others" bucket (deleted/ungrouped channels). */
const OTHERS_COLOUR = '#94a3b8'

const FALLBACK_PALETTE = [
  '#0ea5e9',
  '#22c55e',
  '#a855f7',
  '#f97316',
  '#14b8a6',
  '#eab308',
  '#ec4899',
  '#84cc16',
  '#6366f1',
  '#10b981',
  '#f43f5e',
  '#0891b2',
]

/** Channel names are dynamic, so colours are assigned by list order; the
 * "Others" bucket keeps its stable gray in both the chart and the list. */
function buildChannelColourMap(names: string[]): Record<string, string> {
  const result: Record<string, string> = {}
  let fallbackIdx = 0
  for (const name of names) {
    if (name === 'Others') {
      result[name] = OTHERS_COLOUR
    } else if (result[name] === undefined) {
      result[name] = FALLBACK_PALETTE[fallbackIdx % FALLBACK_PALETTE.length]
      fallbackIdx += 1
    }
  }
  return result
}

const MAX_CHANNELS_IN_LIST = 12

type ChannelsSectionProps = {
  history: ChannelShareSeries
  rows: RankedChannel[]
  period: RankingPeriod
}

/**
 * Combined "Channel Share" card: a 100%-stacked bar chart showing each
 * channel's slice of total token volume, paired below with a two-column
 * channel list.
 */
export function ChannelsSection(props: ChannelsSectionProps) {
  const { t } = useTranslation()
  const { resolvedTheme, themeReady } = useChartTheme()
  const chartTextColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.68)'
      : 'rgba(15, 23, 42, 0.58)'
  const chartGridColor =
    resolvedTheme === 'dark'
      ? 'rgba(255, 255, 255, 0.12)'
      : 'rgba(15, 23, 42, 0.12)'

  // "Others" is an internal bucket value; localise it only at display time.
  const displayChannel = useCallback(
    (name: string) => (name === 'Others' ? t('Others') : name),
    [t]
  )

  const totalTokens = useMemo(
    () => props.rows.reduce((sum, row) => sum + row.total_tokens, 0),
    [props.rows]
  )

  const colourMap = useMemo(
    () =>
      buildChannelColourMap([
        ...props.history.channels.map((channel) => channel.name),
        ...props.rows.map((row) => row.channel_name),
      ]),
    [props.history, props.rows]
  )

  const orderedPoints = useMemo(() => {
    const order = new Map(
      props.history.channels.map((channel, idx) => [channel.name, idx] as const)
    )
    return [...props.history.points].sort((a, b) => {
      const tsCmp = a.ts.localeCompare(b.ts)
      if (tsCmp !== 0) return tsCmp
      return (order.get(a.channel) ?? 999) - (order.get(b.channel) ?? 999)
    })
  }, [props.history])

  const spec = useMemo(() => {
    if (orderedPoints.length === 0) return null
    return {
      type: 'bar' as const,
      data: [{ id: 'channel-share', values: orderedPoints }],
      xField: 'label',
      yField: 'share',
      seriesField: 'channel',
      stack: true,
      paddingInner: 0.12,
      legends: { visible: false },
      color: { specified: colourMap },
      axes: [
        {
          orient: 'bottom',
          label: {
            style: { fill: chartTextColor, fontSize: 10 },
            autoHide: true,
            autoLimit: true,
          },
          tick: { visible: false },
        },
        {
          orient: 'left',
          min: 0,
          max: 1,
          label: {
            formatMethod: (val: number | string) =>
              `${Math.round(Number(val) * 100)}%`,
            style: { fill: chartTextColor, fontSize: 10 },
          },
          grid: {
            visible: true,
            style: { lineDash: [3, 3], stroke: chartGridColor },
          },
        },
      ],
      tooltip: {
        mark: {
          content: [
            {
              key: (datum: Record<string, unknown>) =>
                displayChannel(String(datum?.channel ?? '')),
              value: (datum: Record<string, unknown>) =>
                `${(Number(datum?.share) * 100).toFixed(1)}% · ${formatTokens(Number(datum?.tokens) || 0)}`,
            },
          ],
        },
        dimension: {
          title: {
            value: (datum: Record<string, unknown>) =>
              String(datum?.label ?? ''),
          },
          content: [
            {
              key: (datum: Record<string, unknown>) =>
                displayChannel(String(datum?.channel ?? '')),
              value: (datum: Record<string, unknown>) =>
                Number(datum?.share) || 0,
            },
          ],
          updateContent: (
            array: Array<{ key: string; value: string | number }>
          ) => {
            return array
              .filter((item) => Number(item.value) > 0.001)
              .sort((a, b) => Number(b.value) - Number(a.value))
              .map((item) => ({
                key: item.key,
                value: `${(Number(item.value) * 100).toFixed(1)}%`,
              }))
          },
        },
      },
      animationAppear: { duration: 500 },
    }
  }, [chartGridColor, chartTextColor, colourMap, orderedPoints, displayChannel])

  const visible = props.rows.slice(0, MAX_CHANNELS_IN_LIST)

  // No channel traffic in this period — hide the whole section.
  if (props.rows.length === 0 || totalTokens <= 0) {
    return null
  }

  const half = Math.ceil(visible.length / 2)
  const left = visible.slice(0, half)
  const right = visible.slice(half)

  return (
    <section className='bg-card overflow-hidden rounded-lg border'>
      {/* Chart block ----------------------------------------------------- */}
      <header className='px-5 py-4'>
        <h2 className='text-foreground inline-flex items-center gap-2 text-base font-semibold'>
          <Server className='text-primary size-4' />
          {t('Channel share')}
        </h2>
        <p className='text-muted-foreground mt-1 text-sm'>
          {t(PERIOD_DESCRIPTIONS[props.period])}
        </p>
      </header>

      <div className='px-5 pb-5'>
        <div className='h-60 sm:h-72'>
          {themeReady && spec ? (
            <VChart
              key={`channel-share-${resolvedTheme}-${props.period}`}
              spec={{
                ...spec,
                theme: resolvedTheme === 'dark' ? 'dark' : 'light',
                background: 'transparent',
              }}
              option={VCHART_OPTION}
            />
          ) : (
            <div className='text-muted-foreground/80 flex h-full items-center justify-center text-xs'>
              {t('No history data available')}
            </div>
          )}
        </div>
      </div>

      {/* Channel list block ---------------------------------------------- */}
      <div className='border-t'>
        <header className='px-5 pt-4 pb-2'>
          <h3 className='text-foreground text-sm font-semibold'>
            {t('By channel')}
          </h3>
          <p className='text-muted-foreground/80 mt-0.5 text-xs'>
            {t('Channels ranked by aggregated token volume')}
          </p>
        </header>
        {visible.length === 0 ? (
          <div className='text-muted-foreground/80 px-5 py-8 text-center text-sm'>
            {t('No channel data available')}
          </div>
        ) : (
          <div className='grid grid-cols-1 gap-x-8 px-5 pt-1 pb-4 md:grid-cols-2'>
            <ChannelList
              rows={left}
              colourMap={colourMap}
              displayChannel={displayChannel}
            />
            {right.length > 0 && (
              <ChannelList
                rows={right}
                colourMap={colourMap}
                displayChannel={displayChannel}
              />
            )}
          </div>
        )}
      </div>
    </section>
  )
}

function ChannelList(props: {
  rows: RankedChannel[]
  colourMap: Record<string, string>
  displayChannel: (name: string) => string
}) {
  return (
    <ul>
      {props.rows.map((channel) => (
        <li
          key={channel.channel_name}
          className='flex items-center gap-3 py-2.5'
        >
          <span className='text-muted-foreground/80 w-6 shrink-0 text-right font-mono text-xs tabular-nums'>
            {channel.rank}.
          </span>
          <span
            aria-hidden
            className='size-2.5 shrink-0 rounded-full'
            style={{
              backgroundColor:
                props.colourMap[channel.channel_name] ?? '#94a3b8',
            }}
          />
          <span className='text-foreground min-w-0 flex-1 truncate text-sm font-medium'>
            {props.displayChannel(channel.channel_name)}
          </span>
          <GrowthText value={channel.growth_pct} className='shrink-0 text-xs' />
          <div className='shrink-0 text-right'>
            <div className='text-foreground font-mono text-sm font-semibold tabular-nums'>
              {formatTokens(channel.total_tokens)}
            </div>
            <div className='text-muted-foreground/80 font-mono text-[11px] tabular-nums'>
              {formatShare(channel.share)}
            </div>
          </div>
        </li>
      ))}
    </ul>
  )
}
