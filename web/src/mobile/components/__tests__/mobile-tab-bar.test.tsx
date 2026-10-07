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
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'

import { MobileTabBar } from '@/mobile/components/mobile-tab-bar'

describe('MobileTabBar', () => {
  it('marks the active tab as selected', () => {
    render(<MobileTabBar active='routing' onChange={vi.fn()} />)

    expect(
      screen.getByRole('tab', { name: 'Routing statistics' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Usage' })).toHaveAttribute(
      'aria-selected',
      'false'
    )
  })

  it('reports the tapped tab', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<MobileTabBar active='usage' onChange={onChange} />)

    await user.click(screen.getByRole('tab', { name: 'Channels' }))

    expect(onChange).toHaveBeenCalledWith('channels')
  })

  it('reports the tab confirmed with the keyboard', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<MobileTabBar active='usage' onChange={onChange} />)

    // Every tab is a native button, so it stays in the tab order and confirms
    // with Enter/Space exactly like a click.
    await user.tab()
    await user.tab()
    await user.tab()
    expect(
      screen.getByRole('tab', { name: 'Routing statistics' })
    ).toHaveFocus()

    await user.keyboard('{Enter}')

    expect(onChange).toHaveBeenCalledWith('routing')
  })

  it('exposes five tabs inside a tablist', () => {
    render(<MobileTabBar active='usage' onChange={vi.fn()} />)

    expect(screen.getByRole('tablist')).toBeInTheDocument()
    expect(screen.getAllByRole('tab')).toHaveLength(5)
  })

  it('renders the routing-weights tab and reports it when tapped', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    render(<MobileTabBar active='usage' onChange={onChange} />)

    const tab = screen.getByRole('tab', { name: 'Model routing' })
    expect(tab).toBeInTheDocument()

    await user.click(tab)

    expect(onChange).toHaveBeenCalledWith('routing-weights')
  })
})
