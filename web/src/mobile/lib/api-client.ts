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
import { clearPat, readPat } from '@/mobile/lib/pat-store'

export type ApiErrorCode =
  | 'unauthorized'
  | 'forbidden'
  | 'http'
  | 'network'
  | 'business'

export class ApiError extends Error {
  readonly code: ApiErrorCode
  readonly status: number
  readonly apiCode?: string

  constructor(
    code: ApiErrorCode,
    message: string,
    status: number,
    apiCode?: string
  ) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.status = status
    this.apiCode = apiCode
  }
}

interface ApiEnvelope<T> {
  success: boolean
  message?: string
  code?: string
  data?: T
}

function buildUrl(
  path: string,
  params?: Record<string, string | number | undefined>
): string {
  if (!params) {
    return path
  }
  const query = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined) {
      continue
    }
    query.set(key, String(value))
  }
  const serialized = query.toString()
  return serialized === '' ? path : `${path}?${serialized}`
}

async function request<T>(
  path: string,
  init: RequestInit,
  params?: Record<string, string | number | undefined>
): Promise<T> {
  const token = readPat()
  let response: Response
  try {
    response = await fetch(buildUrl(path, params), {
      ...init,
      credentials: 'omit',
      headers: {
        Accept: 'application/json',
        ...(token === '' ? {} : { Authorization: `Bearer ${token}` }),
        ...init.headers,
      },
    })
  } catch {
    throw new ApiError('network', 'Network request failed', 0)
  }

  if (response.status === 401) {
    clearPat()
    throw new ApiError('unauthorized', 'Access token rejected', 401)
  }
  if (response.status === 403) {
    throw new ApiError(
      'forbidden',
      'Access token has no permission for this action',
      403
    )
  }

  let envelope: ApiEnvelope<T> | null = null
  try {
    envelope = (await response.json()) as ApiEnvelope<T>
  } catch {
    envelope = null
  }

  if (!response.ok) {
    throw new ApiError(
      'http',
      envelope?.message ?? `Request failed with status ${response.status}`,
      response.status,
      envelope?.code
    )
  }
  if (!envelope || envelope.success !== true) {
    throw new ApiError(
      'business',
      envelope?.message ?? 'Request failed',
      response.status,
      envelope?.code
    )
  }
  return envelope.data as T
}

export function mobileApiGet<T>(
  path: string,
  params?: Record<string, string | number | undefined>
): Promise<T> {
  return request<T>(path, { method: 'GET' }, params)
}

export function mobileApiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
}
