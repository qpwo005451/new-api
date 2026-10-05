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
import { useTranslation } from 'react-i18next'

import { formatTimestampRelative } from '@/lib/format'
import { KpiCard } from '@/mobile/components/kpi-card'
import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { ValueRow } from '@/mobile/components/value-row'
import { useModelAvailability } from '@/mobile/features/models/api'
import {
  countByHealth,
  flattenAvailability,
} from '@/mobile/features/models/lib/availability'
import type { AvailabilityHealth } from '@/mobile/types'

const HEALTH_LABEL_KEY: Record<AvailabilityHealth, string> = {
  normal: 'Healthy',
  degraded: 'Degraded',
  unavailable: 'Unavailable',
  unknown: 'Unknown',
}

const MODEL_STATUS_LABEL_KEY: Record<string, string> = {
  available: 'Available',
  limited: 'Limited',
  unavailable: 'Unavailable',
  unknown: 'Unknown',
}

export function ModelsPage() {
  const { t } = useTranslation()
  const availability = useModelAvailability()

  if (availability.isPending) {
    return <MobileLoading />
  }
  if (availability.isError) {
    return (
      <MobileError title={t('Load failed')} description={t('Retry later.')} />
    )
  }
  if (!availability.data.enabled) {
    return (
      <p className='text-muted-foreground px-3 py-6 text-sm'>
        {t('Model monitoring is disabled on this instance.')}
      </p>
    )
  }

  const counts = countByHealth(availability.data.sites)
  const rows = flattenAvailability(availability.data.sites)
  const newestObservation = availability.data.sites.reduce(
    (latest, site) => Math.max(latest, site.latest_observed_at),
    0
  )

  return (
    <div className='space-y-4 px-3 pb-4'>
      <div className='grid grid-cols-2 gap-2'>
        {(Object.keys(HEALTH_LABEL_KEY) as AvailabilityHealth[]).map(
          (health) => (
            <KpiCard
              key={health}
              label={HEALTH_LABEL_KEY[health]}
              value={counts[health]}
              hint='Sites'
            />
          )
        )}
      </div>

      <p className='text-muted-foreground text-[11px]'>
        {t('Latest probe')} {formatTimestampRelative(newestObservation)}
      </p>

      <section>
        <h2 className='px-3 py-1 text-sm font-medium'>{t('Models by site')}</h2>
        <div className='divide-y'>
          {rows.map((row) => (
            <ValueRow
              key={`${row.siteName}/${row.modelName}`}
              label={row.modelName}
              secondary={`${row.siteName}${row.latestFailureType ? ` · ${row.latestFailureType}` : ''}`}
              value={t(MODEL_STATUS_LABEL_KEY[row.status] ?? 'Unknown')}
            />
          ))}
        </div>
      </section>

      {rows.some((row) => row.latestErrorSummary) ? (
        <section>
          <h2 className='px-3 py-1 text-sm font-medium'>
            {t('Latest failures')}
          </h2>
          <ul className='space-y-1 px-3'>
            {rows
              .filter((row) => row.latestErrorSummary)
              .map((row) => (
                <li
                  key={`error-${row.siteName}-${row.modelName}`}
                  className='text-xs'
                >
                  <span className='font-medium'>{row.modelName}</span>{' '}
                  {row.latestErrorSummary}
                </li>
              ))}
          </ul>
        </section>
      ) : null}
    </div>
  )
}
