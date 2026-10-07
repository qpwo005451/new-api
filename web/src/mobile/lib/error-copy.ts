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
import { ApiError } from '@/mobile/lib/api-client'

export interface MobileErrorCopy {
  titleKey: string
  descriptionKey: string
}

/**
 * Maps a failed mobile query to operator-facing copy.
 *
 * The installed PWA runs without browser chrome, so a generic "Load failed /
 * Retry later." leaves the operator with no way to tell a dropped connection
 * (common when a LAN-only console is opened off the network) apart from a
 * gateway problem. Naming the failure lets them fix the right thing.
 */
export function mobileErrorCopy(error: unknown): MobileErrorCopy {
  if (error instanceof ApiError && error.code === 'network') {
    return {
      titleKey: 'Connection failed',
      descriptionKey: 'Network connection failed or server not responding',
    }
  }
  if (error instanceof ApiError && error.code === 'forbidden') {
    return {
      titleKey: 'Administrator access required',
      descriptionKey: 'This page needs an administrator access token.',
    }
  }
  return {
    titleKey: 'Load failed',
    descriptionKey: 'Retry later.',
  }
}
