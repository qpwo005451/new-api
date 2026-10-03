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
import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { MiniBar } from '@/mobile/components/mini-bar'

describe('MiniBar', () => {
  it('renders one segment per entry with proportional width', () => {
    render(
      <MiniBar
        segments={[
          { key: 'a', share: 0.25 },
          { key: 'b', share: 0.75 },
        ]}
      />
    )

    const segments = screen.getAllByTestId('mini-bar-segment')
    expect(segments).toHaveLength(2)
    expect(segments[0]).toHaveStyle({ width: '25%' })
    expect(segments[1]).toHaveStyle({ width: '75%' })
  })

  it('renders nothing for an empty segment list', () => {
    render(<MiniBar segments={[]} />)

    expect(screen.queryAllByTestId('mini-bar-segment')).toHaveLength(0)
  })
})
