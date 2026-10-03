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

import {
  channelStatusLabelKey,
  channelToggleTarget,
} from '@/mobile/features/channels/lib/channel-status'

describe('channelToggleTarget', () => {
  it('disables an enabled channel with the manual-disabled status', () => {
    expect(channelToggleTarget(1)).toBe(2)
  })

  it('enables a manually disabled channel', () => {
    expect(channelToggleTarget(2)).toBe(1)
  })

  // The server rejects status 3 for writes (isManageableChannelStatus only
  // accepts 1 and 2), so an auto-disabled channel can only be enabled.
  it('enables an auto disabled channel instead of sending status 3', () => {
    expect(channelToggleTarget(3)).toBe(1)
  })
})

describe('channelStatusLabelKey', () => {
  it('maps every server status to a distinct label key', () => {
    expect(channelStatusLabelKey(1)).toBe('Enabled')
    expect(channelStatusLabelKey(2)).toBe('Manually disabled')
    expect(channelStatusLabelKey(3)).toBe('Auto disabled')
    expect(channelStatusLabelKey(9)).toBe('Unknown status')
  })
})
