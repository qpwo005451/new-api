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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { mobileApiGet, mobileApiPatch } from '@/mobile/lib/api-client'
import { MOBILE_STALE_TIME } from '@/mobile/lib/query-client'
import type { RequestPolicyOptions } from '@/mobile/types'

export const ROUTING_WEIGHTS_QUERY_KEY = [
  'mobile',
  'routing-weights',
  'request-policy',
] as const

export function useRequestPolicy() {
  return useQuery({
    queryKey: ROUTING_WEIGHTS_QUERY_KEY,
    queryFn: () =>
      mobileApiGet<RequestPolicyOptions>('/api/option/request_policy'),
    staleTime: MOBILE_STALE_TIME.routing,
  })
}

export function useSaveRequestPolicy() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (options: Record<string, string>) =>
      mobileApiPatch<RequestPolicyOptions>('/api/option/request_policy', {
        options,
      }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: ROUTING_WEIGHTS_QUERY_KEY,
      })
    },
  })
}
