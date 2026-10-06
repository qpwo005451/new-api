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
import { describe, expect, it } from 'vitest'

import { ApiError } from '@/mobile/lib/api-client'
import { mobileErrorCopy } from '@/mobile/lib/error-copy'

describe('mobileErrorCopy', () => {
  it('names the connection failure when the request never reached the server', () => {
    expect(
      mobileErrorCopy(new ApiError('network', 'Network request failed', 0))
    ).toEqual({
      titleKey: 'Connection failed',
      descriptionKey: 'Network connection failed or server not responding',
    })
  })

  it('keeps the generic retry copy for HTTP and business failures', () => {
    expect(
      mobileErrorCopy(new ApiError('http', 'Request failed', 500))
    ).toEqual({
      titleKey: 'Load failed',
      descriptionKey: 'Retry later.',
    })
    expect(mobileErrorCopy(new Error('boom'))).toEqual({
      titleKey: 'Load failed',
      descriptionKey: 'Retry later.',
    })
  })
})
