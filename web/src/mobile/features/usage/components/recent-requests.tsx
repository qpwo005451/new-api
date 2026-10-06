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

import {
  formatLogQuota,
  formatTimestampRelative,
  formatTokens,
  formatUseTime,
} from '@/lib/format'
import { MobileEmpty } from '@/mobile/components/mobile-empty'
import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { ValueRow } from '@/mobile/components/value-row'
import { useRecentLogs } from '@/mobile/features/usage/api'
import { mobileErrorCopy } from '@/mobile/lib/error-copy'
import type { TimeRange, UsageScope } from '@/mobile/types'

interface RecentRequestsProps {
  scope: UsageScope
  range: TimeRange
}

export function RecentRequests(props: RecentRequestsProps) {
  const { t } = useTranslation()
  const logs = useRecentLogs(props.scope, props.range, 2)
  // The paged log endpoints answer `{ items, total, ... }`. The optional chain
  // and array guard keep the list from throwing if a proxy or an error page
  // returns a bare array where the envelope is expected.
  const items = Array.isArray(logs.data?.items) ? logs.data.items : []

  if (logs.isPending) {
    return <MobileLoading />
  }
  // A failed log request must not be reported as "no requests": that reads as
  // a healthy empty range while the panel is actually broken.
  if (logs.isError) {
    const errorCopy = mobileErrorCopy(logs.error)
    return (
      <MobileError
        title={t(errorCopy.titleKey)}
        description={t(errorCopy.descriptionKey)}
        onRetry={() => void logs.refetch()}
      />
    )
  }
  if (items.length === 0) {
    return (
      <MobileEmpty
        title={t('No recent requests')}
        description={t('Nothing to show for this range.')}
      />
    )
  }

  return (
    <div className='divide-y'>
      {items.map((row) => (
        <ValueRow
          key={row.id}
          label={row.model_name}
          secondary={`${formatTimestampRelative(row.created_at)} · ${formatUseTime(row.use_time)}`}
          value={`${formatTokens(row.prompt_tokens + row.completion_tokens)} · ${formatLogQuota(row.quota)}`}
        />
      ))}
    </div>
  )
}
