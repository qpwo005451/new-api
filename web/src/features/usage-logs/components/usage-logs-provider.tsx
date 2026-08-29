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
/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useState,
  type ReactNode,
} from 'react'

import { useIsAdmin } from '@/hooks/use-admin'

import { getClientAliases, upsertClientAlias } from '../api'
import type { ChannelAffinityInfo } from '../types'

export type LogsViewScope = 'all' | 'self'

interface UsageLogsContextValue {
  selectedUserId: number | null
  setSelectedUserId: (userId: number | null) => void
  userInfoDialogOpen: boolean
  setUserInfoDialogOpen: (open: boolean) => void
  affinityTarget: ChannelAffinityInfo | null
  setAffinityTarget: (target: ChannelAffinityInfo | null) => void
  affinityDialogOpen: boolean
  setAffinityDialogOpen: (open: boolean) => void
  sensitiveVisible: boolean
  setSensitiveVisible: (visible: boolean) => void
  viewScope: LogsViewScope
  setViewScope: (scope: LogsViewScope) => void
  clientAliases: Record<string, string>
  saveClientAlias: (
    userAgent: string,
    name: string
  ) => Promise<{ success: boolean; message?: string }>
  markClientDialogUa: string | null
  setMarkClientDialogUa: (userAgent: string | null) => void
}

const UsageLogsContext = createContext<UsageLogsContextValue | undefined>(
  undefined
)

export function UsageLogsProvider({ children }: { children: ReactNode }) {
  const isAdmin = useIsAdmin()
  const [selectedUserId, setSelectedUserId] = useState<number | null>(null)
  const [userInfoDialogOpen, setUserInfoDialogOpen] = useState(false)
  const [affinityTarget, setAffinityTarget] =
    useState<ChannelAffinityInfo | null>(null)
  const [affinityDialogOpen, setAffinityDialogOpen] = useState(false)
  const [sensitiveVisible, setSensitiveVisible] = useState(true)
  const [viewScope, setViewScope] = useState<LogsViewScope>('all')
  const [clientAliases, setClientAliases] = useState<Record<string, string>>({})
  const [markClientDialogUa, setMarkClientDialogUa] = useState<string | null>(
    null
  )

  useEffect(() => {
    if (!isAdmin) return
    let cancelled = false
    getClientAliases()
      .then((res) => {
        if (!cancelled && res.success && res.data) {
          setClientAliases(res.data)
        }
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [isAdmin])

  const saveClientAlias = useCallback(
    async (userAgent: string, name: string) => {
      const res = await upsertClientAlias(userAgent, name)
      if (res.success && res.data) {
        setClientAliases(res.data)
      }
      return res
    },
    []
  )

  return (
    <UsageLogsContext.Provider
      value={{
        selectedUserId,
        setSelectedUserId,
        userInfoDialogOpen,
        setUserInfoDialogOpen,
        affinityTarget,
        setAffinityTarget,
        affinityDialogOpen,
        setAffinityDialogOpen,
        sensitiveVisible,
        setSensitiveVisible,
        viewScope,
        setViewScope,
        clientAliases,
        saveClientAlias,
        markClientDialogUa,
        setMarkClientDialogUa,
      }}
    >
      {children}
    </UsageLogsContext.Provider>
  )
}

export function useUsageLogsContext() {
  const context = useContext(UsageLogsContext)
  if (!context) {
    throw new Error('useUsageLogsContext must be used within UsageLogsProvider')
  }
  return context
}

/**
 * Resolves the effective admin scope for usage logs: whether the current
 * user is allowed to view all users' logs (`canManageScope`), and whether
 * their current view preference (`viewScope`) has that scope active
 * (`isAdminView`). Data fetching and admin-only UI should key off
 * `isAdminView` rather than raw role, so an admin who switches to "only
 * mine" is treated exactly like a regular user for that view.
 */
export function useLogsViewScope() {
  const canManageScope = useIsAdmin()
  const { viewScope, setViewScope } = useUsageLogsContext()

  return {
    canManageScope,
    viewScope,
    setViewScope,
    isAdminView: canManageScope && viewScope === 'all',
  }
}
