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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError, mobileApiGet, mobileApiPost } from '@/mobile/lib/api-client'
import { readPat, writePat } from '@/mobile/lib/pat-store'

const VALID_PAT = 'a'.repeat(29)

function mockJsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

describe('api-client', () => {
  beforeEach(() => {
    window.localStorage.clear()
    vi.unstubAllGlobals()
  })

  it('sends the stored token as a bearer credential and unwraps data', async () => {
    writePat(VALID_PAT)
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        mockJsonResponse({ success: true, data: { quota: 7 } })
      )
    vi.stubGlobal('fetch', fetchMock)

    await expect(
      mobileApiGet<{ quota: number }>('/api/log/stat', {
        start_timestamp: 1,
        end_timestamp: 2,
        empty: undefined,
      })
    ).resolves.toEqual({ quota: 7 })

    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/log/stat?start_timestamp=1&end_timestamp=2')
    expect(init.headers.Authorization).toBe(`Bearer ${VALID_PAT}`)
    expect(init.method).toBe('GET')
  })

  it('posts a json body', async () => {
    writePat(VALID_PAT)
    const fetchMock = vi
      .fn()
      .mockResolvedValue(mockJsonResponse({ success: true, data: true }))
    vi.stubGlobal('fetch', fetchMock)

    await mobileApiPost('/api/channel/3/status', { status: 2 })

    const [, init] = fetchMock.mock.calls[0]
    expect(init.method).toBe('POST')
    expect(init.body).toBe('{"status":2}')
    expect(init.headers['Content-Type']).toBe('application/json')
  })

  it('throws unauthorized and clears the token on 401', async () => {
    writePat(VALID_PAT)
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockJsonResponse({ success: false, message: 'invalid' }, 401)
        )
    )

    await expect(mobileApiGet('/api/log/stat')).rejects.toMatchObject({
      code: 'unauthorized',
      status: 401,
    })
    expect(readPat()).toBe('')
  })

  it('throws forbidden on 403 without clearing the token', async () => {
    writePat(VALID_PAT)
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          mockJsonResponse({ success: false, message: 'no permission' }, 403)
        )
    )

    await expect(mobileApiGet('/api/channel/')).rejects.toMatchObject({
      code: 'forbidden',
      status: 403,
    })
    expect(readPat()).toBe(VALID_PAT)
  })

  it('throws business with the server message when success is false', async () => {
    writePat(VALID_PAT)
    // A Response body can only be read once, and this case asserts both the
    // classification and the message on separate requests, so every call needs
    // its own response object.
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockImplementation(() =>
          Promise.resolve(
            mockJsonResponse({ success: false, message: 'time range too wide' })
          )
        )
    )

    await expect(mobileApiGet('/api/data/self')).rejects.toMatchObject({
      code: 'business',
      apiCode: undefined,
    })
    await expect(mobileApiGet('/api/data/self')).rejects.toThrow(
      'time range too wide'
    )
  })

  it('throws network when fetch rejects', async () => {
    writePat(VALID_PAT)
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')))

    const error = await mobileApiGet('/api/log/stat').catch(
      (cause: unknown) => cause
    )
    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ code: 'network' })
  })
})
