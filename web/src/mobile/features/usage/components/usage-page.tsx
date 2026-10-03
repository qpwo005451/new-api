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
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { formatQuotaWithCurrency } from '@/lib/currency'
import { formatNumber, formatTokens } from '@/lib/format'
import { KpiCard } from '@/mobile/components/kpi-card'
import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { ValueRow } from '@/mobile/components/value-row'
import {
  useLiveRate,
  useUsageAggregate,
  useUsageRanking,
} from '@/mobile/features/usage/api'
import { RecentRequests } from '@/mobile/features/usage/components/recent-requests'
import {
  aggregateTotals,
  rankRows,
  resolveTimeRange,
} from '@/mobile/features/usage/lib/usage-summary'
import type { TimeRangePreset, UsageScope } from '@/mobile/types'

const PRESETS: readonly TimeRangePreset[] = ['today', '7d', '30d']
const PRESET_LABEL_KEY: Record<TimeRangePreset, string> = {
  today: 'Today',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
}

export function UsagePage() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [scope, setScope] = useState<UsageScope>('self')
  const [preset, setPreset] = useState<TimeRangePreset>('today')
  const range = resolveTimeRange(preset, Math.floor(Date.now() / 1000))

  const aggregate = useUsageAggregate(scope, range)
  const rate = useLiveRate(scope, range)
  const ranking = useUsageRanking(scope, range, scope === 'all')

  // `/api/data/self` and `/api/data/` answer with a list of quota rows. A bare
  // array guard keeps a proxy or an error envelope from breaking the render.
  const rows = Array.isArray(aggregate.data) ? aggregate.data : []
  const totals = aggregateTotals(rows)
  const modelRanking = rankRows(rows, 'model_name', 5)
  const userRanking = Array.isArray(ranking.data)
    ? rankRows(ranking.data, 'username', 5)
    : []

  // The scope and range toggles stay interactive while a range loads so the
  // operator can switch away from a slow or failing query.
  const controls = (
    <>
      <div role='group' aria-label={t('Usage scope')} className='flex gap-2'>
        {(['self', 'all'] as const).map((option) => (
          <button
            key={option}
            type='button'
            aria-pressed={scope === option}
            className={
              scope === option
                ? 'text-foreground text-sm font-medium'
                : 'text-muted-foreground text-sm'
            }
            onClick={() => setScope(option)}
          >
            {t(option === 'self' ? 'Mine' : 'All')}
          </button>
        ))}
      </div>

      <div role='group' aria-label={t('Time range')} className='flex gap-2'>
        {PRESETS.map((option) => (
          <button
            key={option}
            type='button'
            aria-pressed={preset === option}
            onClick={() => setPreset(option)}
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
    </>
  )

  if (aggregate.isPending) {
    return (
      <div className='space-y-4 px-3 pb-4'>
        {controls}
        <MobileLoading />
      </div>
    )
  }
  if (aggregate.isError) {
    return (
      <div className='space-y-4 px-3 pb-4'>
        {controls}
        <MobileError
          title={t('Load failed')}
          description={t('Retry later.')}
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
          value={totals.requests}
          hint={PRESET_LABEL_KEY[preset]}
        />
        <KpiCard
          label='Tokens'
          value={formatTokens(totals.tokens)}
          hint={PRESET_LABEL_KEY[preset]}
        />
        <KpiCard
          label='Cost'
          value={formatQuotaWithCurrency(totals.quota, { locale })}
          hint={PRESET_LABEL_KEY[preset]}
        />
        <KpiCard
          label='Current rate'
          value={
            rate.data
              ? `${formatNumber(rate.data.rpm, locale)} / ${formatTokens(rate.data.tpm)}`
              : '—'
          }
          hint='Last 60 seconds'
        />
      </div>

      <p className='text-muted-foreground text-[11px]'>
        {t('Summary data updates every few minutes.')}
      </p>

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>{t('By model')}</h2>
        <div className='divide-y'>
          {modelRanking.map((row) => (
            <ValueRow
              key={row.key}
              label={row.key}
              value={formatTokens(row.tokens)}
              secondary={formatQuotaWithCurrency(row.quota, { locale })}
            />
          ))}
        </div>
      </section>

      {scope === 'all' ? (
        <section>
          <h2 className='px-3 py-1 text-sm font-medium'>{t('By user')}</h2>
          <div className='divide-y'>
            {userRanking.map((row) => (
              <ValueRow
                key={row.key}
                label={row.key}
                value={formatTokens(row.tokens)}
                secondary={formatQuotaWithCurrency(row.quota, { locale })}
              />
            ))}
          </div>
        </section>
      ) : null}

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>
          {t('Recent requests')}
        </h2>
        <RecentRequests scope={scope} range={range} />
      </section>
    </div>
  )
}
