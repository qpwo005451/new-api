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
import { queryOptions } from '@tanstack/react-query'

import {
  STATUS_QUERY_KEY,
  syncStatusToSystemConfig,
  type StatusData,
} from '@/lib/status-config'
import { mobileApiGet } from '@/mobile/lib/api-client'

export { STATUS_QUERY_KEY }

/**
 * The mobile console's `/api/status` query.
 *
 * `/api/status` has no auth middleware, so this runs before the PAT gate and
 * `mobileApiGet` sends no `Authorization` header until a token is stored. It
 * shares the desktop query key so both consoles agree on the payload shape.
 */
export const mobileStatusQueryOptions = queryOptions({
  queryKey: STATUS_QUERY_KEY,
  queryFn: async (): Promise<StatusData> => {
    const status = await mobileApiGet<StatusData>('/api/status')
    syncStatusToSystemConfig(status)
    return status
  },
  // Data becomes stale after 5 minutes
  staleTime: 5 * 60 * 1000,
  // Cache expires after 30 minutes
  gcTime: 30 * 60 * 1000,
})
