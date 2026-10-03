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

import { CHANNEL_STATUS_FILTER } from '@/mobile/features/channels/lib/channel-status'
import { mobileApiGet, mobileApiPost } from '@/mobile/lib/api-client'
import { MOBILE_STALE_TIME } from '@/mobile/lib/query-client'
import type { ChannelFilters, ChannelListResult } from '@/mobile/types'

const CHANNEL_PAGE_SIZE = 50

export function useChannels(filters: ChannelFilters) {
  const keyword = filters.name.trim()
  const status =
    filters.statusFilter === CHANNEL_STATUS_FILTER.all
      ? undefined
      : filters.statusFilter

  return useQuery({
    queryKey: ['mobile', 'channels', keyword, filters.statusFilter],
    queryFn: () =>
      // `GET /api/channel/` has no name filter; only `/api/channel/search`
      // matches on the channel name, via `keyword`, with the same envelope.
      keyword === ''
        ? mobileApiGet<ChannelListResult>('/api/channel/', {
            p: 1,
            page_size: CHANNEL_PAGE_SIZE,
            status,
          })
        : mobileApiGet<ChannelListResult>('/api/channel/search', {
            keyword,
            p: 1,
            page_size: CHANNEL_PAGE_SIZE,
            status,
          }),
    staleTime: MOBILE_STALE_TIME.channels,
  })
}

export function useToggleChannelStatus() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (input: { id: number; status: number }) =>
      mobileApiPost<boolean>(`/api/channel/${input.id}/status`, {
        status: input.status,
      }),
    onSettled: async () => {
      await queryClient.invalidateQueries({ queryKey: ['mobile', 'channels'] })
    },
  })
}
