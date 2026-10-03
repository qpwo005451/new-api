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

import { MobileEmpty } from '@/mobile/components/mobile-empty'
import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'

/**
 * The mobile console cannot reuse the desktop state components: those drag
 * `@tanstack/react-router`, `motion/react` and `lucide-react` into the first
 * screen bundle. These primitives must stay icon-free.
 */

describe('MobileLoading', () => {
  it('announces the loading state without rendering an icon', () => {
    const { container } = render(<MobileLoading />)

    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.getByText('Loading...')).toBeInTheDocument()
    expect(container.querySelector('svg')).toBeNull()
  })
})

describe('MobileEmpty', () => {
  it('renders the title and the description without any icon', () => {
    const { container } = render(
      <MobileEmpty title='No channels' description='Nothing to show.' />
    )

    expect(screen.getByText('No channels')).toBeInTheDocument()
    expect(screen.getByText('Nothing to show.')).toBeInTheDocument()
    expect(container.querySelector('svg')).toBeNull()
  })
})

describe('MobileError', () => {
  it('renders a retry button that invokes onRetry when provided', async () => {
    const user = userEvent.setup()
    const onRetry = vi.fn()
    render(
      <MobileError
        title='Load failed'
        description='Retry later.'
        onRetry={onRetry}
      />
    )

    expect(screen.getByText('Load failed')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Retry' }))

    expect(onRetry).toHaveBeenCalledTimes(1)
  })

  it('renders no button at all when onRetry is omitted', () => {
    const { container } = render(
      <MobileError title='Load failed' description='Retry later.' />
    )

    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(container.querySelector('svg')).toBeNull()
  })
})
