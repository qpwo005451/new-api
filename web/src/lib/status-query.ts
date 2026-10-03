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
import { queryOptions, type QueryClient } from '@tanstack/react-query'

import { getStatus } from '@/lib/api'
import {
  STATUS_QUERY_KEY,
  STATUS_STORAGE_KEY,
  mapStatusDataToConfig,
  readCachedStatus,
  syncStatusToSystemConfig,
  type StatusData,
} from '@/lib/status-config'

// The transport-free half of the status contract lives in `status-config.ts`
// so the mobile console can reuse it without pulling axios in. Re-exported here
// so existing desktop consumers keep the `@/lib/status-query` import path.
export {
  STATUS_QUERY_KEY,
  STATUS_STORAGE_KEY,
  mapStatusDataToConfig,
  readCachedStatus,
}
export type { StatusData }

/**
 * Single source of truth for `/api/status`.
 *
 * `/api/status` is entirely global on the backend: every field is read from
 * in-memory option maps under a read lock, with no user context and no auth
 * middleware on the route. That makes it safe to share one cache entry across
 * every consumer — branding, nav module gates, and the setup guard.
 *
 * Anything that needs status must go through `statusQueryOptions` so React
 * Query can dedupe. Calling `getStatus()` directly re-introduces the duplicate
 * requests this module exists to collapse.
 */
async function fetchStatus(): Promise<StatusData | null> {
  const status = (await getStatus()) as StatusData | null

  if (status) {
    syncStatusToSystemConfig(status)
  }

  return status
}

export const statusQueryOptions = queryOptions({
  queryKey: STATUS_QUERY_KEY,
  queryFn: fetchStatus,
  // Data becomes stale after 5 minutes
  staleTime: 5 * 60 * 1000,
  // Cache expires after 30 minutes
  gcTime: 30 * 60 * 1000,
})

/**
 * Await status from the shared cache.
 *
 * Use this when a cached snapshot can be shown during a background refresh,
 * such as system configuration loading. Concurrent callers share one request.
 * Navigation guards use `fetchQuery(statusQueryOptions)` instead, because
 * redirects must wait for stale or invalidated access flags to refresh.
 *
 * Resolution rules, which are `ensureQueryData`'s and not `staleTime`'s:
 * - No cached entry: fetches and awaits the response.
 * - Cached entry, fresh: resolves from cache, no network.
 * - Cached entry, stale: resolves from cache *immediately* and kicks off a
 *   background refresh (`revalidateIfStale`). Consumers must subscribe to the
 *   updated query or system-config store to observe the refreshed data.
 *
 * The React Query cache is memory-only (no persister is installed), so a hard
 * reload always starts from an empty cache and fetches.
 */
export async function ensureStatus(
  queryClient: QueryClient
): Promise<StatusData | null> {
  return queryClient.ensureQueryData({
    ...statusQueryOptions,
    revalidateIfStale: true,
  })
}
