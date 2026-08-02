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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import type { ColumnDef } from '@tanstack/react-table'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DataTablePage,
  DataTableRow,
  useDataTable,
} from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { cn } from '@/lib/utils'

import { cancelInFlightLog } from '../api'
import {
  DEFAULT_LOGS_DATA,
  LOG_TYPE_ALL_VALUE,
  LOG_TYPE_ENUM,
} from '../constants'
import type { UsageLog } from '../data/schema'
import { useColumnsByCategory } from '../lib/columns'
import { parseLogOther } from '../lib/format'
import { fetchLogsByCategory } from '../lib/utils'
import type { LogCategory } from '../types'
import { CommonLogsFilterBar } from './common-logs-filter-bar'
import { DetailsDialog } from './dialogs/details-dialog'
import { TaskLogsFilterBar } from './task-logs-filter-bar'
import { UsageLogsMobileList } from './usage-logs-mobile-card'
import { useLogsViewScope } from './usage-logs-provider'

const route = getRouteApi('/_authenticated/usage-logs/$section')

const logTypeRowTint: Record<number, string> = {
  [LOG_TYPE_ENUM.ERROR]: 'bg-rose-50/40 dark:bg-rose-950/20',
  [LOG_TYPE_ENUM.REFUND]: 'bg-blue-50/30 dark:bg-blue-950/15',
  [LOG_TYPE_ENUM.PENDING]: 'bg-warning/5',
}

// Warning tint for logs where a quota conversion saturated (admin-only marker).
// Takes precedence over the per-type tint since it flags a billing anomaly.
const quotaSaturationRowTint = 'bg-amber-50/60 dark:bg-amber-950/25'

function getColumnVisibilityStorageKey(
  logCategory: LogCategory,
  isAdmin: boolean
): string {
  return `usage-logs:${logCategory}:${isAdmin ? 'admin' : 'user'}:column-visibility`
}

function deserializeLogTypeFilter(value: unknown): unknown[] {
  let values: unknown[] = []
  if (Array.isArray(value)) {
    values = value
  } else if (value) {
    values = [value]
  }
  return values.filter((item) => String(item) !== LOG_TYPE_ALL_VALUE)
}

interface UsageLogsTableProps {
  logCategory: LogCategory
}

export function UsageLogsTable({ logCategory }: UsageLogsTableProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { isAdminView: isAdmin } = useLogsViewScope()
  const isMobile = useMediaQuery('(max-width: 640px)')
  const searchParams = route.useSearch()

  const {
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 20 : 100 },
    globalFilter: { enabled: false },
    columnFilters: [
      {
        columnId: 'created_at',
        searchKey: 'type',
        type: 'array' as const,
        deserialize: deserializeLogTypeFilter,
      },
      { columnId: 'model_name', searchKey: 'model', type: 'string' as const },
      { columnId: 'token_name', searchKey: 'token', type: 'string' as const },
      { columnId: 'group', searchKey: 'group', type: 'string' as const },
      ...(isAdmin
        ? [
            {
              columnId: 'channel',
              searchKey: 'channel',
              type: 'string' as const,
            },
            {
              columnId: 'username',
              searchKey: 'username',
              type: 'string' as const,
            },
          ]
        : []),
    ],
  })

  const [autoRefresh, setAutoRefresh] = useState(true)
  const [selectedLogId, setSelectedLogId] = useState<number | null>(null)
  const [selectedLogSnapshot, setSelectedLogSnapshot] =
    useState<UsageLog | null>(null)
  const [cancelTarget, setCancelTarget] = useState<UsageLog | null>(null)

  const { data, isLoading } = useQuery({
    queryKey: [
      'logs',
      logCategory,
      isAdmin,
      pagination.pageIndex + 1,
      pagination.pageSize,
      columnFilters,
      searchParams,
      t,
    ],
    refetchInterval: logCategory === 'common' && autoRefresh ? 3000 : false,
    queryFn: async () => {
      const result = await fetchLogsByCategory({
        logCategory,
        isAdmin,
        page: pagination.pageIndex + 1,
        pageSize: pagination.pageSize,
        searchParams,
        columnFilters,
      })

      if (!result?.success) {
        toast.error(result?.message || t('Failed to load logs'))
        return DEFAULT_LOGS_DATA
      }

      return result.data || DEFAULT_LOGS_DATA
    },
    placeholderData: (previousData, previousQuery) => {
      if (previousQuery?.queryKey[1] === logCategory) {
        return previousData
      }
      return undefined
    },
  })

  const logs = data?.items || []
  const isCommon = logCategory === 'common'

  const liveSelectedLog =
    selectedLogId == null
      ? null
      : ((logs as UsageLog[]).find((log) => log.id === selectedLogId) ?? null)
  const selectedLog = liveSelectedLog ?? selectedLogSnapshot

  useEffect(() => {
    if (liveSelectedLog) {
      setSelectedLogSnapshot(liveSelectedLog)
    }
  }, [liveSelectedLog])

  const handleViewDetails = useCallback((log: UsageLog) => {
    setSelectedLogId(log.id)
    setSelectedLogSnapshot(log)
  }, [])
  const handleCancelRequest = useCallback((log: UsageLog) => {
    setCancelTarget(log)
  }, [])
  const commonColumnActions = useMemo(
    () => ({
      onViewDetails: handleViewDetails,
      onCancelRequest: handleCancelRequest,
    }),
    [handleCancelRequest, handleViewDetails]
  )
  const columns = useColumnsByCategory(
    logCategory,
    isAdmin,
    commonColumnActions
  )
  const isLoadingData = isLoading

  const cancelMutation = useMutation({
    mutationFn: (logId: number) => cancelInFlightLog(logId),
    onSuccess: () => {
      toast.success(t('Cancellation requested; the client can retry shortly'))
      setCancelTarget(null)
      queryClient.invalidateQueries({ queryKey: ['logs'] })
    },
  })

  const { table } = useDataTable({
    data: logs as Record<string, unknown>[],
    columns: columns as ColumnDef<Record<string, unknown>>[],
    columnFilters,
    columnVisibilityStorageKey: getColumnVisibilityStorageKey(
      logCategory,
      isAdmin
    ),
    pagination,
    enableRowSelection: false,
    getRowId: (row) => String(row.id),
    onPaginationChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total || 0,
    ensurePageInRange,
  })

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns as ColumnDef<Record<string, unknown>>[]}
        isLoading={isLoadingData}
        emptyTitle={t('No Logs Found')}
        emptyDescription={t(
          'No usage logs available. Logs will appear here once API calls are made.'
        )}
        skeletonKeyPrefix='usage-log-skeleton'
        applyHeaderSize
        tableClassName={cn(
          '[&_[data-slot=table]]:text-[13px] [&_[data-slot=table]_td]:text-[13px] [&_[data-slot=table]_td_*]:text-[13px] [&_[data-slot=table]_th]:text-[13px] [&_[data-slot=table]_th_*]:text-[13px]'
        )}
        mobile={
          <UsageLogsMobileList
            table={table}
            isLoading={isLoadingData}
            logCategory={logCategory}
          />
        }
        toolbar={
          isCommon ? (
            <CommonLogsFilterBar
              table={table}
              autoRefresh={autoRefresh}
              onAutoRefreshChange={setAutoRefresh}
            />
          ) : (
            <TaskLogsFilterBar table={table} logCategory={logCategory} />
          )
        }
        renderRow={(row, helpers) => {
          const logType = (row.original as Record<string, unknown>).type as
            | number
            | undefined
          let tintClass =
            isCommon && logType != null ? (logTypeRowTint[logType] ?? '') : ''
          if (isCommon && isAdmin) {
            const other = parseLogOther(
              ((row.original as Record<string, unknown>).other as string) ?? ''
            )
            if (other?.admin_info?.quota_saturation) {
              tintClass = quotaSaturationRowTint
            }
          }

          return (
            <DataTableRow
              key={row.id}
              row={row}
              className={cn('transition-colors', tintClass)}
              getColumnClassName={(columnId) =>
                helpers.getCellClassName(columnId, isCommon ? 'py-2' : 'py-3.5')
              }
            />
          )
        }}
      />
      {isCommon && selectedLog && (
        <DetailsDialog
          log={selectedLog}
          isAdmin={isAdmin}
          open={selectedLogId != null}
          onOpenChange={(open) => {
            if (open) return
            setSelectedLogId(null)
            setSelectedLogSnapshot(null)
          }}
        />
      )}
      <ConfirmDialog
        open={cancelTarget != null}
        onOpenChange={(open) => {
          if (!open && !cancelMutation.isPending) {
            setCancelTarget(null)
          }
        }}
        title={t('Cancel in-flight request?')}
        desc={t(
          'The upstream call will be stopped and the client will receive a retryable response when possible.'
        )}
        confirmText={t('Cancel request')}
        destructive
        isLoading={cancelMutation.isPending}
        handleConfirm={() => {
          if (cancelTarget) {
            cancelMutation.mutate(cancelTarget.id)
          }
        }}
      />
    </>
  )
}
