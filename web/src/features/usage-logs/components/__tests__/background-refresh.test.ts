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

import {
  startUsageLogAutoRefresh,
  USAGE_LOG_REFRESH_INTERVAL_MS,
} from '../usage-log-auto-refresh'

type ScheduledInterval = {
  callback: () => void
  delay: number
}

function createIntervalScheduler() {
  let nextTimerId = 1
  const intervals = new Map<number, ScheduledInterval>()

  return {
    scheduler: {
      setInterval(callback: () => void, delay: number) {
        const timerId = nextTimerId
        nextTimerId += 1
        intervals.set(timerId, { callback, delay })
        return timerId
      },
      clearInterval(timerId: number) {
        intervals.delete(timerId)
      },
    },
    tick() {
      for (const interval of intervals.values()) {
        interval.callback()
      }
    },
    getIntervals() {
      return [...intervals.values()]
    },
  }
}

async function flushPromises() {
  await Promise.resolve()
  await Promise.resolve()
}

describe('usage log background refresh', () => {
  test('refreshes the active list and stats queries on every interval', async () => {
    const calls: unknown[] = []
    const fakeQueryClient = {
      refetchQueries(filters: unknown) {
        calls.push(filters)
        return Promise.resolve()
      },
    }
    const intervalScheduler = createIntervalScheduler()

    const stop = startUsageLogAutoRefresh(
      fakeQueryClient,
      intervalScheduler.scheduler
    )

    assert.deepEqual(intervalScheduler.getIntervals(), [
      {
        callback: intervalScheduler.getIntervals()[0]?.callback,
        delay: USAGE_LOG_REFRESH_INTERVAL_MS,
      },
    ])

    intervalScheduler.tick()
    await flushPromises()
    intervalScheduler.tick()
    await flushPromises()

    assert.deepEqual(calls, [
      { queryKey: ['logs'], type: 'active' },
      { queryKey: ['usage-logs-stats'], type: 'active' },
      { queryKey: ['logs'], type: 'active' },
      { queryKey: ['usage-logs-stats'], type: 'active' },
    ])

    stop()
    assert.equal(intervalScheduler.getIntervals().length, 0)
  })

  test('does not overlap refresh cycles when the previous request is pending', async () => {
    const pendingRefreshes: Array<() => void> = []
    let calls = 0
    const fakeQueryClient = {
      refetchQueries() {
        calls += 1
        return new Promise<void>((resolve) => {
          pendingRefreshes.push(resolve)
        })
      },
    }
    const intervalScheduler = createIntervalScheduler()

    startUsageLogAutoRefresh(fakeQueryClient, intervalScheduler.scheduler)
    intervalScheduler.tick()
    intervalScheduler.tick()

    assert.equal(calls, 2)

    for (const resolve of pendingRefreshes.splice(0)) {
      resolve()
    }
    await flushPromises()
    intervalScheduler.tick()

    assert.equal(calls, 4)
  })

  test('disables row entrance animation for the live usage log table', async () => {
    const stylesheet = await readFile(
      path.resolve(import.meta.dirname, '../../../../styles/index.css'),
      'utf8'
    )

    assert.match(
      stylesheet,
      /\.usage-logs-live-table \[data-slot='table'\] tbody tr\s*\{\s*animation: none;/
    )
  })
})
