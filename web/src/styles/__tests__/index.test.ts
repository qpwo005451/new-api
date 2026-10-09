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
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import path from 'node:path'

import { describe, test } from 'vitest'

describe('global stylesheet', () => {
  test('drops the browser number stepper but keeps keyboard stepping', async () => {
    const stylesheet = await readFile(
      path.resolve(import.meta.dirname, '../index.css'),
      'utf8'
    )

    // `appearance: textfield` removes the field's spinner box; the two
    // webkit pseudo elements are the actual arrows.
    assert.match(
      stylesheet,
      /input\[type='number'\]\s*\{\s*appearance:\s*textfield;/
    )
    assert.match(
      stylesheet,
      /input\[type='number'\]::-webkit-outer-spin-button,\s*input\[type='number'\]::-webkit-inner-spin-button\s*\{\s*appearance:\s*none;/
    )
  })
})
