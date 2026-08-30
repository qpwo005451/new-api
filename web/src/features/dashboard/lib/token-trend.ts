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
import type { TokenTrendPoint } from '@/features/dashboard/types'

export interface TokenTrendSeries {
  time: string
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
  cacheHitRate: number
}

// Reduce hourly buckets into chart rows. prompt_tokens already contains cache
// tokens, so plain input is the remainder after subtracting both cache series.
export function buildTokenTrendSeries(
  points: TokenTrendPoint[]
): TokenTrendSeries[] {
  return points.map((point) => {
    const cacheRead = point.cache_read
    const cacheWrite = point.cache_write
    const input = Math.max(point.prompt_tokens - cacheRead - cacheWrite, 0)
    const promptTotal = point.prompt_tokens
    return {
      time: formatBucketLabel(point.created_at),
      input,
      output: point.completion_tokens,
      cacheRead,
      cacheWrite,
      cacheHitRate:
        promptTotal > 0
          ? Number(((cacheRead / promptTotal) * 100).toFixed(1))
          : 0,
    }
  })
}

function formatBucketLabel(bucketTs: number): string {
  const date = new Date(bucketTs * 1000)
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  return `${month}-${day} ${hours}:00`
}

export function formatCompactTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`
  return String(value)
}
