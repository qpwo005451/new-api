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
const PAT_STORAGE_KEY = 'newapi_mobile_pat'

// The server generates access tokens with common.GenerateRandomKey(29 + rand(4)),
// so anything outside 29..32 characters is a truncated paste.
const PAT_LENGTH_MIN = 29
const PAT_LENGTH_MAX = 32

export function isPlausiblePat(value: string): boolean {
  const trimmed = value.trim()
  return trimmed.length >= PAT_LENGTH_MIN && trimmed.length <= PAT_LENGTH_MAX && !/\s/.test(trimmed)
}

export function readPat(): string {
  try {
    return window.localStorage.getItem(PAT_STORAGE_KEY)?.trim() ?? ''
  } catch {
    return ''
  }
}

export function writePat(pat: string): void {
  try {
    window.localStorage.setItem(PAT_STORAGE_KEY, pat.trim())
  } catch {
    // A browser that blocks storage still works for the current tab; the
    // operator only has to paste the token again after a reload.
  }
}

export function clearPat(): void {
  try {
    window.localStorage.removeItem(PAT_STORAGE_KEY)
  } catch {
    // See writePat.
  }
}
