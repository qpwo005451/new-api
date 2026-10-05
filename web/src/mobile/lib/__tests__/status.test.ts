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
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { STATUS_QUERY_KEY, mobileStatusQueryOptions } from '@/mobile/lib/status'
import { useSystemConfigStore } from '@/stores/system-config-store'

function mockJsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

/**
 * `/api/status` has no auth middleware, so the mobile query must work before
 * the PAT gate and hydrate the currency config every money helper reads.
 */
describe('mobileStatusQueryOptions', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.unstubAllGlobals()
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    useSystemConfigStore.setState(useSystemConfigStore.getInitialState(), true)
  })

  it('hydrates the currency config from /api/status without an Authorization header', async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      mockJsonResponse({
        success: true,
        data: { quota_per_unit: 765432, quota_display_type: 'CNY' },
      })
    )
    vi.stubGlobal('fetch', fetchMock)

    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    })
    const data = await queryClient.fetchQuery(mobileStatusQueryOptions)

    expect(STATUS_QUERY_KEY).toEqual(['status'])
    expect(data).toEqual({ quota_per_unit: 765432, quota_display_type: 'CNY' })
    expect(useSystemConfigStore.getState().config.currency.quotaPerUnit).toBe(
      765432
    )
    expect(
      useSystemConfigStore.getState().config.currency.quotaDisplayType
    ).toBe('CNY')

    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/status')
    expect(init.headers.Authorization).toBeUndefined()
  })
})
