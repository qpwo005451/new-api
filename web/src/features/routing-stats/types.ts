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
export interface RoutingModelChannelStat {
  model_name: string
  channel_id: number
  channel_name: string
  requests: number
  errors: number
  avg_use_time: number
  /** Effective priority channel selection used; absent when the channel is gone. */
  priority?: number
  /** Effective weight channel selection used; absent when the channel is gone. */
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

/** Time range and bucket width of the switch trend. */
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
  /** Requests whose decision trail was inspected for the switch figures. */
  scanned: number
  /** True when the window held more requests than the decision-trail scan cap. */
  truncated: boolean
  switched: number
  switched_success: number
  switched_failed: number
  sticky: number
  switches: RoutingSwitchStat[]
  switch_reasons: RoutingReasonStat[]
  affinity_by_rule: RoutingAffinityStat[]
}

/** One live session binding: a session currently pinned to a channel. */
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
