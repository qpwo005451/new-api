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

import { useQuery } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/design-system/button'
import { StatusBadge, type StatusVariant } from '@/components/status-badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { listSystemTasks } from '@/features/system-settings/api'
import type { SystemTaskStatus } from '@/features/system-settings/types'
import { formatTimestampRelative } from '@/lib/format'

const statusVariant: Record<SystemTaskStatus, StatusVariant> = {
  pending: 'warning',
  running: 'info',
  succeeded: 'success',
  failed: 'destructive',
}

function isActive(status: SystemTaskStatus): boolean {
  return status === 'pending' || status === 'running'
}

export function MonitorTasksPanel() {
  const { t } = useTranslation()
  const tasksQuery = useQuery({
    queryKey: ['model-monitor', 'tasks'],
    queryFn: async () => {
      const response = await listSystemTasks(20)
      if (!response.success || !Array.isArray(response.data)) {
        throw new Error(response.message || t('Failed to load monitor tasks'))
      }
      return response.data.filter((task) => task.type === 'model_monitor')
    },
    retry: false,
    refetchInterval: (query) =>
      query.state.data?.some((task) => isActive(task.status)) ? 8000 : false,
  })
  const tasks = tasksQuery.data ?? []

  return (
    <Card>
      <CardHeader className='flex-row items-start justify-between gap-3'>
        <div>
          <CardTitle>{t('Monitor tasks')}</CardTitle>
          <CardDescription>
            {t('Recent scheduled and manually requested monitor runs.')}
          </CardDescription>
        </div>
        <Button
          type='button'
          variant='outline'
          size='icon'
          onClick={() => void tasksQuery.refetch()}
          disabled={tasksQuery.isFetching}
          aria-label={t('Refresh')}
        >
          <RefreshCw
            className={tasksQuery.isFetching ? 'animate-spin' : undefined}
            aria-hidden='true'
          />
        </Button>
      </CardHeader>
      <CardContent>
        {tasksQuery.isError && (
          <p className='text-destructive text-sm'>
            {tasksQuery.error instanceof Error
              ? tasksQuery.error.message
              : t('Failed to load monitor tasks')}
          </p>
        )}
        {!tasksQuery.isError && tasks.length === 0 && (
          <p className='text-muted-foreground text-sm'>
            {t('No monitor tasks yet.')}
          </p>
        )}
        {!tasksQuery.isError && tasks.length > 0 && (
          <div className='space-y-2'>
            {tasks.map((task) => (
              <div
                key={task.task_id}
                className='flex flex-wrap items-center justify-between gap-2 rounded-md border px-3 py-2 text-sm'
              >
                <div className='min-w-0'>
                  <p className='font-medium'>{t('Model monitor run')}</p>
                  <p className='text-muted-foreground truncate text-xs'>
                    {formatTimestampRelative(task.updated_at)}
                    {task.error ? ` · ${task.error}` : ''}
                  </p>
                </div>
                <StatusBadge variant={statusVariant[task.status]}>
                  {t(task.status)}
                </StatusBadge>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
