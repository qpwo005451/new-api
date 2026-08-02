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
import type { QueryClient } from '@tanstack/react-query'

export const USAGE_LOG_REFRESH_INTERVAL_MS = 3000

type UsageLogQueryClient = Pick<QueryClient, 'refetchQueries'>

type IntervalScheduler = {
  setInterval: (callback: () => void, delay: number) => number
  clearInterval: (timerId: number) => void
}

export async function refreshUsageLogQueries(
  queryClient: UsageLogQueryClient
): Promise<void> {
  await Promise.all([
    queryClient.refetchQueries({
      queryKey: ['logs'],
      type: 'active',
    }),
    queryClient.refetchQueries({
      queryKey: ['usage-logs-stats'],
      type: 'active',
    }),
  ])
}

export function startUsageLogAutoRefresh(
  queryClient: UsageLogQueryClient,
  scheduler: IntervalScheduler = window
): () => void {
  let refreshInProgress = false
  const timerId = scheduler.setInterval(() => {
    if (refreshInProgress) return

    refreshInProgress = true
    void refreshUsageLogQueries(queryClient).finally(() => {
      refreshInProgress = false
    })
  }, USAGE_LOG_REFRESH_INTERVAL_MS)

  return () => scheduler.clearInterval(timerId)
}
