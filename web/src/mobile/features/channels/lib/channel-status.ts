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
export const CHANNEL_STATUS = {
  enabled: 1,
  manuallyDisabled: 2,
  autoDisabled: 3,
} as const

/**
 * The list endpoint only understands three status filters (`parseStatusFilter`):
 * `1` matches enabled channels, `0` matches both manual and automatic disables,
 * and any other value means "no filter". The mobile chips therefore expose
 * three options and send the server's own values.
 */
export const CHANNEL_STATUS_FILTER = {
  all: -1,
  enabled: CHANNEL_STATUS.enabled,
  disabled: 0,
} as const

const STATUS_LABEL_KEY: Record<number, string> = {
  [CHANNEL_STATUS.enabled]: 'Enabled',
  [CHANNEL_STATUS.manuallyDisabled]: 'Manually disabled',
  [CHANNEL_STATUS.autoDisabled]: 'Auto disabled',
}

// The server only accepts 1 and 2 for status writes (isManageableChannelStatus),
// so an auto-disabled channel can be enabled but never re-disabled through this
// control.
export function channelToggleTarget(status: number): number {
  return status === CHANNEL_STATUS.enabled
    ? CHANNEL_STATUS.manuallyDisabled
    : CHANNEL_STATUS.enabled
}

export function channelStatusLabelKey(status: number): string {
  return STATUS_LABEL_KEY[status] ?? 'Unknown status'
}
