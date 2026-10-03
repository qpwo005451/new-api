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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatPercent, formatUseTime } from '@/lib/format'
import { KpiCard } from '@/mobile/components/kpi-card'
import { MiniBar } from '@/mobile/components/mini-bar'
import { MobileEmpty } from '@/mobile/components/mobile-empty'
import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { ValueRow } from '@/mobile/components/value-row'
import {
  useAffinityBindings,
  useRoutingStats,
} from '@/mobile/features/routing/api'
import {
  summarizeRoutingShare,
  topSwitches,
} from '@/mobile/features/routing/lib/routing-share'
import { resolveTimeRange } from '@/mobile/features/usage/lib/usage-summary'
import { ApiError } from '@/mobile/lib/api-client'
import type { TimeRangePreset } from '@/mobile/types'

const PRESETS = [
  'today',
  '7d',
  '30d',
] as const satisfies readonly TimeRangePreset[]
const PRESET_LABEL_KEY: Record<TimeRangePreset, string> = {
  today: 'Today',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
}

export function RoutingPage() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [preset, setPreset] = useState<TimeRangePreset>('today')
  // The window is frozen to an anchor captured on mount and on every preset
  // switch. `range` feeds the query keys, so resolving `Date.now()` during
  // render makes every response arrival mint a new key (new cache entry ->
  // pending again -> another request) and hammer the backend. Task 8 review.
  const [anchor, setAnchor] = useState(() => Math.floor(Date.now() / 1000))
  const range = useMemo(
    () => resolveTimeRange(preset, anchor),
    [preset, anchor]
  )
  const selectPreset = (next: TimeRangePreset) => {
    setPreset(next)
    setAnchor(Math.floor(Date.now() / 1000))
  }

  const stats = useRoutingStats(range)
  const affinity = useAffinityBindings()

  const controls = (
    <div role='group' aria-label={t('Time range')} className='flex gap-2'>
      {PRESETS.map((option) => (
        <button
          key={option}
          type='button'
          aria-pressed={preset === option}
          onClick={() => selectPreset(option)}
          className={
            preset === option
              ? 'text-foreground text-sm font-medium'
              : 'text-muted-foreground text-sm'
          }
        >
          {t(PRESET_LABEL_KEY[option])}
        </button>
      ))}
    </div>
  )

  if (stats.isPending) {
    return (
      <div className='space-y-4 px-3 pb-4'>
        {controls}
        <MobileLoading />
      </div>
    )
  }
  if (stats.isError) {
    // Both routing endpoints require an administrator token, so a 403 is a
    // role problem, not a transient failure; say so instead of showing blank.
    const forbidden =
      stats.error instanceof ApiError && stats.error.code === 'forbidden'
    return (
      <div className='space-y-4 px-3 pb-4'>
        {controls}
        <MobileError
          title={
            forbidden
              ? t('Administrator access required')
              : t('Load failed')
          }
          description={
            forbidden
              ? t('This page needs an administrator access token.')
              : t('Retry later.')
          }
        />
      </div>
    )
  }

  // A proxy or an error envelope can answer with the wrong shape; guarding the
  // list fields keeps the page from crashing on `.map` of a non-array.
  const rows = Array.isArray(stats.data.by_model_channel)
    ? stats.data.by_model_channel
    : []
  const switchRows = Array.isArray(stats.data.switches)
    ? stats.data.switches
    : []
  const affinityRows = Array.isArray(stats.data.affinity_by_rule)
    ? stats.data.affinity_by_rule
    : []

  const models = summarizeRoutingShare(rows)
  const switches = topSwitches(switchRows, 5)
  const stickyRequests = affinityRows.reduce(
    (total, row) => total + row.sticky_requests,
    0
  )

  if (models.length === 0 && switches.length === 0 && stickyRequests === 0) {
    return (
      <div className='space-y-4 px-3 pb-4'>
        {controls}
        <MobileEmpty
          title={t('No Data')}
          description={t(
            'Run requests, or widen the time window, to collect routing statistics.'
          )}
        />
      </div>
    )
  }

  return (
    <div className='space-y-4 px-3 pb-4'>
      {controls}

      <div className='grid grid-cols-2 gap-2'>
        <KpiCard
          label='Requests'
          value={models.reduce((total, model) => total + model.requests, 0)}
          hint={PRESET_LABEL_KEY[preset]}
        />
        <KpiCard
          label='Switched requests'
          value={formatNumber(stats.data.switched, locale)}
          hint={PRESET_LABEL_KEY[preset]}
        />
      </div>

      {stats.data.truncated ? (
        <p role='status' className='text-muted-foreground px-3 text-[11px]'>
          {t('Some switches in this window were not scanned.')}
        </p>
      ) : null}

      <section className='space-y-3'>
        <h2 className='px-3 py-1 text-sm font-medium'>
          {t('Traffic by model')}
        </h2>
        {models.length === 0 ? (
          <MobileEmpty
            title={t('No Data')}
            description={t('No model received requests in this window.')}
            className='min-h-0 py-4'
          />
        ) : (
          models.map((model) => (
            <article key={model.modelName} className='space-y-2'>
              <div className='flex items-baseline justify-between'>
                <h3 className='text-sm font-medium'>{model.modelName}</h3>
                <p className='text-muted-foreground text-xs'>
                  {formatNumber(model.requests, locale)} ·{' '}
                  {formatNumber(model.errors, locale)} {t('errors')} ·{' '}
                  {formatUseTime(model.avgUseTime)}
                </p>
              </div>
              <MiniBar
                segments={model.channels.map((channel) => ({
                  key: String(channel.channelId),
                  share:
                    model.requests === 0
                      ? 0
                      : channel.requests / model.requests,
                }))}
              />
              <div className='divide-y'>
                {model.channels.map((channel) => (
                  <ValueRow
                    key={channel.channelId}
                    label={channel.channelName || `#${channel.channelId}`}
                    secondary={
                      channel.configuredShare === null
                        ? t('Configured weight unknown')
                        : `${t('Configured weight')} ${formatPercent(channel.configuredShare * 100)}`
                    }
                    value={formatPercent(
                      (model.requests === 0
                        ? 0
                        : channel.requests / model.requests) * 100
                    )}
                  />
                ))}
              </div>
            </article>
          ))
        )}
      </section>

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>
          {t('Sticky sessions')}
        </h2>
        {affinity.isError ? (
          <p className='text-muted-foreground px-3 py-2 text-xs'>
            {t('Affinity bindings are unavailable.')}
          </p>
        ) : (
          <ValueRow
            label={t('Sticky sessions')}
            value={formatNumber(stickyRequests, locale)}
            secondary={
              typeof affinity.data?.total === 'number'
                ? `${formatNumber(affinity.data.total, locale)} ${t('keys')}`
                : undefined
            }
          />
        )}
      </section>

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>
          {t('Channel switches')}
        </h2>
        {switches.length === 0 ? (
          <MobileEmpty
            title={t('No Data')}
            description={t('No channel switch was recorded in this window.')}
            className='min-h-0 py-4'
          />
        ) : (
          <div className='divide-y'>
            {switches.map((row) => (
              <ValueRow
                key={`${row.from}-${row.to}`}
                label={`${row.from} → ${row.to}`}
                value={formatNumber(row.count, locale)}
              />
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
