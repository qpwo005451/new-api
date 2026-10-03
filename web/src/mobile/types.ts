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
export type MobileTab = 'usage' | 'models' | 'routing' | 'channels'

/** Bottom-navigation order of the mobile console tabs. */
export const MOBILE_TABS = [
  'usage',
  'models',
  'routing',
  'channels',
] as const satisfies readonly MobileTab[]

export type UsageScope = 'self' | 'all'

export type TimeRangePreset = 'today' | '7d' | '30d'

export interface TimeRange {
  start: number
  end: number
}

export interface UsageTotals {
  requests: number
  tokens: number
  quota: number
}

export interface RankRow {
  key: string
  requests: number
  tokens: number
  quota: number
}

export interface QuotaDataRow {
  model_name?: string
  username?: string
  created_at: number
  count: number
  quota: number
  token_used: number
}

export interface LogStat {
  quota: number
  rpm: number
  tpm: number
}

export interface UsageLogRow {
  id: number
  created_at: number
  model_name: string
  token_name?: string
  prompt_tokens: number
  completion_tokens: number
  quota: number
  use_time: number
  is_stream: boolean
  type: number
}

export interface PagedResult<T> {
  items: T[]
  total: number
  page: number
  page_size: number
}

export type AvailabilityHealth = 'normal' | 'degraded' | 'unavailable' | 'unknown'

export interface MonitorSiteModel {
  model_name: string
  status: string
  latest_status: string
  latest_failure_type?: string
  latest_error_summary?: string
  weight: number
  stale: boolean
}

export interface MonitorSiteSummary {
  score: number
  health: string
  models: MonitorSiteModel[]
}

export interface MonitorSiteResponse {
  site: { id: number; name: string; enabled: boolean }
  summary: MonitorSiteSummary
  channel_ids: number[]
  latest_observed_at: number
  freshness_seconds?: number
}

export interface ModelMonitorSummary {
  enabled: boolean
  sites: MonitorSiteResponse[]
}

export interface RoutingModelChannelStat {
  model_name: string
  channel_id: number
  channel_name: string
  requests: number
  errors: number
  avg_use_time: number
  priority?: number
  weight?: number
}

export interface RoutingSwitchStat {
  from: number
  to: number
  count: number
}

export interface RoutingReasonStat {
  reason: string
  count: number
}

export interface RoutingAffinityStat {
  rule_name: string
  sticky_requests: number
  distinct_keys: number
}

export interface RoutingStatsWindow {
  start: number
  end: number
  bucket_seconds: number
}

export interface RoutingTrendPoint {
  timestamp: number
  requests: number
  switched: number
}

export interface RoutingStats {
  requests: number
  errors: number
  by_model_channel: RoutingModelChannelStat[]
  window: RoutingStatsWindow
  trend: RoutingTrendPoint[]
  // The trail scan truncates, so switched/scanned/switches only cover the rows
  // that were scanned (Task 10 review Minor-2).
  scanned: number
  truncated: boolean
  switched: number
  switched_success: number
  switched_failed: number
  sticky: number
  switches: RoutingSwitchStat[]
  switch_reasons: RoutingReasonStat[]
  affinity_by_rule: RoutingAffinityStat[]
}

export interface ChannelAffinityBinding {
  rule_name: string
  model_name: string
  using_group: string
  key_hint: string
  key_fingerprint: string
  channel_id: number
  channel_name: string
}

export interface ChannelAffinityBindings {
  enabled: boolean
  total: number
  unknown: number
  truncated: boolean
  entries: ChannelAffinityBinding[]
}

export interface AvailabilityRow {
  siteName: string
  modelName: string
  status: string
  latestFailureType?: string
  latestErrorSummary?: string
  stale: boolean
}
