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
import { describe, test } from 'node:test'

const componentRoot = new URL('../', import.meta.url)

describe('usage log background refresh', () => {
  test('keeps the table visually stable while polling', async () => {
    const source = await readFile(
      new URL('usage-logs-table.tsx', componentRoot),
      'utf8'
    )

    assert.equal(source.includes('isFetching={isFetching}'), false)
    assert.equal(
      source.includes(
        "refetchInterval: logCategory === 'common' && autoRefresh ? 3000 : false"
      ),
      true
    )
  })

  test('refreshes usage RPM and TPM with the same auto-refresh toggle', async () => {
    const statsSource = await readFile(
      new URL('common-logs-stats.tsx', componentRoot),
      'utf8'
    )
    const filterSource = await readFile(
      new URL('common-logs-filter-bar.tsx', componentRoot),
      'utf8'
    )

    assert.equal(
      statsSource.includes(
        'refetchInterval: props.autoRefresh === true ? 3000 : false'
      ),
      true
    )
    assert.equal(
      filterSource.includes('<CommonLogsStats autoRefresh={autoRefresh} />'),
      true
    )
  })
})
