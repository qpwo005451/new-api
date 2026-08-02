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

const featureRoot = new URL('../../', import.meta.url)

describe('model monitor alert settings', () => {
  test('exposes alert configuration and test actions in the management UI', async () => {
    const componentSource = await readFile(
      new URL('components/alert-settings.tsx', featureRoot),
      'utf8'
    )
    const apiSource = await readFile(new URL('api.ts', featureRoot), 'utf8')

    assert.equal(
      componentSource.includes('updateModelMonitorAlertConfig'),
      true
    )
    assert.equal(componentSource.includes('testModelMonitorAlerts'), true)
    assert.equal(apiSource.includes("'/api/model-monitor/alerts/config'"), true)
    assert.equal(apiSource.includes("'/api/model-monitor/alerts/test'"), true)
  })

  test('supports site channel exact-model and prefix rule editing', async () => {
    const source = await readFile(
      new URL('components/alert-settings.tsx', featureRoot),
      'utf8'
    )

    assert.equal(source.includes("value='prefix'"), true)
    assert.equal(source.includes("value='exact'"), true)
    assert.equal(source.includes('channel_id'), true)
    assert.equal(source.includes('site_id'), true)
  })
})
