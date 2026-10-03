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
import { QueryClient } from '@tanstack/react-query'

import { ApiError } from '@/mobile/lib/api-client'

// Aggregated usage comes from quota_data, which the export task refreshes on
// DataExportInterval (5 minutes by default); live rate figures are cheap to
// refetch, and channel state is what the operator acts on.
export const MOBILE_STALE_TIME = {
  usage: 60_000,
  availability: 60_000,
  routing: 120_000,
  channels: 30_000,
} as const

export const mobileQueryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: false,
      retry: (failureCount, error) => {
        if (error instanceof ApiError && error.code !== 'network') {
          return false
        }
        return failureCount < 2
      },
    },
  },
})
