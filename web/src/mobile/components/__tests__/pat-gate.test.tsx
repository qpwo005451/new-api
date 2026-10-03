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
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { PatGate } from '@/mobile/components/pat-gate'
import { readPat } from '@/mobile/lib/pat-store'

describe('PatGate', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('asks for a token when none is stored', () => {
    render(
      <PatGate onReady={vi.fn()}>
        <p>console</p>
      </PatGate>
    )

    expect(screen.getByLabelText('Access token')).toBeInTheDocument()
    expect(screen.queryByText('console')).not.toBeInTheDocument()
  })

  it('rejects a token that is too short without storing it', async () => {
    const user = userEvent.setup()
    render(
      <PatGate onReady={vi.fn()}>
        <p>console</p>
      </PatGate>
    )

    await user.type(screen.getByLabelText('Access token'), 'short')
    await user.click(
      screen.getByRole('button', { name: 'Verify and continue' })
    )

    expect(
      screen.getByText(
        'The token looks too short. Check that you copied all of it.'
      )
    ).toBeInTheDocument()
    expect(readPat()).toBe('')
  })

  it('stores a plausible token and reveals the console', async () => {
    const user = userEvent.setup()
    const onReady = vi.fn()
    render(
      <PatGate onReady={onReady}>
        <p>console</p>
      </PatGate>
    )

    await user.type(screen.getByLabelText('Access token'), 'a'.repeat(29))
    await user.click(
      screen.getByRole('button', { name: 'Verify and continue' })
    )

    expect(readPat()).toBe('a'.repeat(29))
    expect(onReady).toHaveBeenCalledWith('a'.repeat(29))
  })

  it('skips the gate when a token is already stored', () => {
    window.localStorage.setItem('newapi_mobile_pat', 'b'.repeat(29))

    render(
      <PatGate onReady={vi.fn()}>
        <p>console</p>
      </PatGate>
    )

    expect(screen.getByText('console')).toBeInTheDocument()
  })
})
