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
*/

export type ModelMonitorHealth =
  | 'normal'
  | 'degraded'
  | 'unavailable'
  | 'unknown'

export type ModelMonitorStatus =
  | 'available'
  | 'limited'
  | 'unavailable'
  | 'unknown'

export type ModelMonitorSetting = {
  enabled: boolean
  auto_probe_enabled: boolean
  auto_probe_interval_minutes: number
  unknown_grace_minutes: number
  pricing_import_user_ids: number[]
}

export type ModelMonitorTarget = {
  id?: number
  model_name: string
  endpoint_type: string
  weight: number
  enabled: boolean
}

export type ModelMonitorSiteConfig = {
  id?: number
  name: string
  site_type: 'newapi' | 'sub2api'
  pricing_group?: string
  enabled: boolean
  channel_ids: number[]
  targets: ModelMonitorTarget[]
}

export type ModelMonitorConfig = {
  setting: ModelMonitorSetting
  sites: ModelMonitorSiteConfig[]
}

export type ModelMonitorEffectiveModel = {
  model_name: string
  status: ModelMonitorStatus
  latest_status: ModelMonitorStatus
  latest_failure_type?: string
  latest_error_summary?: string
  weight: number
  stale: boolean
}

export type ModelMonitorSiteSummary = {
  score: number
  health: ModelMonitorHealth
  models: ModelMonitorEffectiveModel[]
}

export type ModelMonitorSite = {
  id: number
  name: string
  site_type: string
  pricing_group?: string
  pricing_sync_status?: string
  pricing_sync_error?: string
  pricing_synced_at?: number
  enabled: boolean
}

export type ModelMonitorObservation = {
  id: number
  target_id: number
  channel_id: number
  model_name: string
  status: ModelMonitorStatus
  source: 'active' | 'passive'
  failure_type: string
  error_summary?: string
  first_response_ms?: number | null
  total_duration_ms: number
  observed_at: number
}

export type ModelMonitorAggregateHourly = {
  id: number
  channel_id: number
  hour_start: number
  observation_count: number
  available_count: number
  limited_count: number
  unavailable_count: number
  unknown_count: number
  availability_basis_points: number
  first_response_p95_ms?: number
  total_duration_p95_ms: number
  failure_counts: string
}

export type ModelMonitorPricingMetadata = {
  snapshot_id: number
  source: string
  version: string
  model_family: string
  modality: string
  billing_class: string
  captured_at: number
}

export type ModelMonitorModelDetail = {
  site: ModelMonitorSite
  target: ModelMonitorTarget
  summary: ModelMonitorSiteSummary
  pricing?: ModelMonitorPricingMetadata
  aggregates: ModelMonitorAggregateHourly[]
  observations: ModelMonitorObservation[]
}

export type ModelMonitorSiteResponse = {
  site: ModelMonitorSite
  summary: ModelMonitorSiteSummary
  channel_ids: number[]
  latest_observed_at: number
  freshness_seconds?: number
  observations?: ModelMonitorObservation[]
}

export type ApiResponse<T> = {
  success: boolean
  message?: string
  data?: T
}
