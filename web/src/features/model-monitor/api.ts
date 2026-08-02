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

import { api } from '@/lib/api'

import type {
  ApiResponse,
  ModelMonitorAlertConfig,
  ModelMonitorAlertTestResult,
  ModelMonitorConfig,
  ModelMonitorModelDetail,
  ModelMonitorSiteResponse,
} from './types'

export async function getModelMonitorConfig(): Promise<
  ApiResponse<ModelMonitorConfig>
> {
  const response = await api.get<ApiResponse<ModelMonitorConfig>>(
    '/api/model-monitor/config'
  )
  return response.data
}

export async function updateModelMonitorConfig(
  config: ModelMonitorConfig
): Promise<ApiResponse<ModelMonitorConfig>> {
  const response = await api.put<ApiResponse<ModelMonitorConfig>>(
    '/api/model-monitor/config',
    config
  )
  return response.data
}

export async function getModelMonitorSites(): Promise<
  ApiResponse<ModelMonitorSiteResponse[]>
> {
  const response = await api.get<ApiResponse<ModelMonitorSiteResponse[]>>(
    '/api/model-monitor/sites'
  )
  return response.data
}

export async function getModelMonitorSite(
  siteId: number
): Promise<ApiResponse<ModelMonitorSiteResponse>> {
  const response = await api.get<ApiResponse<ModelMonitorSiteResponse>>(
    `/api/model-monitor/sites/${siteId}`
  )
  return response.data
}

export async function getModelMonitorModel(
  siteId: number,
  modelName: string
): Promise<ApiResponse<ModelMonitorModelDetail>> {
  const response = await api.get<ApiResponse<ModelMonitorModelDetail>>(
    `/api/model-monitor/sites/${siteId}/models/${encodeURIComponent(modelName)}`
  )
  return response.data
}

export async function enqueueModelMonitorRun(): Promise<
  ApiResponse<{ task_id: string; created: boolean }>
> {
  const response = await api.post<
    ApiResponse<{ task_id: string; created: boolean }>
  >('/api/model-monitor/runs')
  return response.data
}

export async function getModelMonitorAlertConfig(): Promise<
  ApiResponse<ModelMonitorAlertConfig>
> {
  const response = await api.get<ApiResponse<ModelMonitorAlertConfig>>(
    '/api/model-monitor/alerts/config'
  )
  return response.data
}

export async function updateModelMonitorAlertConfig(
  config: ModelMonitorAlertConfig
): Promise<ApiResponse<ModelMonitorAlertConfig>> {
  const response = await api.put<ApiResponse<ModelMonitorAlertConfig>>(
    '/api/model-monitor/alerts/config',
    config
  )
  return response.data
}

export async function testModelMonitorAlerts(): Promise<
  ApiResponse<ModelMonitorAlertTestResult>
> {
  const response = await api.post<ApiResponse<ModelMonitorAlertTestResult>>(
    '/api/model-monitor/alerts/test'
  )
  return response.data
}
