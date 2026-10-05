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
import { DEFAULT_SYSTEM_NAME, DEFAULT_LOGO } from '@/lib/constants'
import {
  useSystemConfigStore,
  type CurrencyConfig,
  type CurrencyDisplayType,
  type SystemConfig,
  DEFAULT_CURRENCY_CONFIG,
} from '@/stores/system-config-store'

/**
 * Pure, transport-free half of the `/api/status` contract.
 *
 * This module exists so the mobile console can hydrate the system-config store
 * without importing `@/lib/status-query`, which pulls in `@/lib/api` → axios.
 * Nothing here may import the HTTP client.
 */

export const STATUS_QUERY_KEY = ['status'] as const

export const STATUS_STORAGE_KEY = 'status'

/** Status payload shape — loose on purpose; the backend map is open-ended. */
export type StatusData = Record<string, unknown>

/** Coerce a status field to a number, keeping `fallback` for unusable values. */
function toNumber(value: unknown, fallback: number): number {
  if (typeof value === 'number' && !Number.isNaN(value)) return value
  if (typeof value === 'string') {
    const parsed = Number(value)
    if (!Number.isNaN(parsed)) return parsed
  }
  return fallback
}

/**
 * Map `/api/status` response data to our persisted system config structure
 */
export function mapStatusDataToConfig(
  data: StatusData | undefined | null
): Partial<SystemConfig> {
  if (!data) return {}

  const quotaDisplayType =
    (data.quota_display_type as CurrencyDisplayType | undefined) ??
    DEFAULT_CURRENCY_CONFIG.quotaDisplayType

  const currency: CurrencyConfig = {
    displayInCurrency:
      (data.display_in_currency as boolean | undefined) ??
      DEFAULT_CURRENCY_CONFIG.displayInCurrency,
    quotaDisplayType,
    quotaPerUnit: toNumber(
      data.quota_per_unit,
      DEFAULT_CURRENCY_CONFIG.quotaPerUnit
    ),
    usdExchangeRate: toNumber(
      data.usd_exchange_rate,
      DEFAULT_CURRENCY_CONFIG.usdExchangeRate
    ),
    customCurrencySymbol:
      (data.custom_currency_symbol as string | undefined)?.trim() ||
      DEFAULT_CURRENCY_CONFIG.customCurrencySymbol,
    customCurrencyExchangeRate: toNumber(
      data.custom_currency_exchange_rate,
      DEFAULT_CURRENCY_CONFIG.customCurrencyExchangeRate
    ),
  }

  return {
    systemName: (data.system_name as string | undefined) || DEFAULT_SYSTEM_NAME,
    logo: (data.logo as string | undefined) || DEFAULT_LOGO,
    footerHtml: data.footer_html as string | undefined,
    demoSiteEnabled: data.demo_site_enabled as boolean | undefined,
    displayTokenStatEnabled: data.display_token_stat_enabled as
      | boolean
      | undefined,
    currency,
  }
}

/** Read the last known status from localStorage (survives reload, may be stale). */
export function readCachedStatus(): StatusData | null {
  try {
    if (typeof window === 'undefined') return null
    const raw = window.localStorage.getItem(STATUS_STORAGE_KEY)
    return raw ? (JSON.parse(raw) as StatusData) : null
  } catch {
    return null
  }
}

/** Persist the latest status so the next cold start can render before fetching. */
export function writeCachedStatus(status: StatusData | null): void {
  try {
    if (typeof window !== 'undefined' && status) {
      window.localStorage.setItem(STATUS_STORAGE_KEY, JSON.stringify(status))
    }
  } catch {
    /* Storage can be unavailable in private mode. */
  }
}

/**
 * Apply a `/api/status` payload to the system-config store and cache it.
 *
 * Owns both side effects of a successful read so every transport — the desktop
 * axios client and the mobile fetch client — stays consistent.
 */
export function syncStatusToSystemConfig(status: StatusData): void {
  try {
    useSystemConfigStore.getState().setConfig(mapStatusDataToConfig(status))
  } catch (err) {
    if (import.meta.env.DEV) {
      // eslint-disable-next-line no-console
      console.warn('[status] Failed to sync status to system config', err)
    }
  }
  writeCachedStatus(status)
}
