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
import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { toIntlLocale } from '@/i18n/languages'
import { formatCurrencyFromUSD } from '@/lib/currency'
import { MobileEmpty } from '@/mobile/components/mobile-empty'
import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { ValueRow } from '@/mobile/components/value-row'
import {
  useChannels,
  useToggleChannelStatus,
} from '@/mobile/features/channels/api'
import {
  CHANNEL_STATUS,
  CHANNEL_STATUS_FILTER,
  channelStatusLabelKey,
  channelToggleTarget,
} from '@/mobile/features/channels/lib/channel-status'
import { ApiError } from '@/mobile/lib/api-client'
import type { ChannelFilters, ChannelRow } from '@/mobile/types'

const STATUS_FILTERS = [
  { value: CHANNEL_STATUS_FILTER.all, label: 'All' },
  { value: CHANNEL_STATUS_FILTER.enabled, label: 'Enabled' },
  { value: CHANNEL_STATUS_FILTER.disabled, label: 'Disabled' },
] as const

export function ChannelsPage() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const [filters, setFilters] = useState<ChannelFilters>({
    name: '',
    statusFilter: CHANNEL_STATUS_FILTER.all,
  })
  const [pending, setPending] = useState<ChannelRow | null>(null)
  const channels = useChannels(filters)
  const toggle = useToggleChannelStatus()

  const confirmToggle = async () => {
    if (!pending) {
      return
    }
    const target = channelToggleTarget(pending.status)
    try {
      await toggle.mutateAsync({ id: pending.id, status: target })
      toast.success(
        target === CHANNEL_STATUS.enabled
          ? t('Channel enabled')
          : t('Channel disabled')
      )
    } catch (error) {
      // A 403 is a role problem, not a transient failure; say so instead of
      // showing the generic retry message.
      toast.error(
        error instanceof ApiError && error.code === 'forbidden'
          ? t('Administrator access required')
          : t('Could not update the channel status.')
      )
    } finally {
      setPending(null)
    }
  }

  // A proxy or an error envelope can answer with the wrong shape; guarding the
  // list keeps the page from crashing on `.map` of a non-array.
  const items = Array.isArray(channels.data?.items) ? channels.data.items : []
  const enabling = pending
    ? channelToggleTarget(pending.status) === CHANNEL_STATUS.enabled
    : false

  let content: ReactNode
  if (channels.isPending) {
    content = <MobileLoading />
  } else if (channels.isError) {
    content = (
      <MobileError title={t('Load failed')} description={t('Retry later.')} />
    )
  } else if (items.length === 0) {
    content = (
      <MobileEmpty
        title={t('No channels')}
        description={t('Nothing to show for this range.')}
      />
    )
  } else {
    content = (
      <ul className='divide-y'>
        {items.map((channel) => (
          <li key={channel.id} className='flex min-h-14 items-center gap-3'>
            <div className='min-w-0 flex-1'>
              <ValueRow
                label={channel.name}
                secondary={`#${channel.id} · ${channel.group} · ${t(channelStatusLabelKey(channel.status))}`}
                value={formatCurrencyFromUSD(channel.balance, { locale })}
              />
            </div>
            <Switch
              aria-label={`${t(channelStatusLabelKey(channel.status))} ${channel.name}`}
              checked={channel.status === CHANNEL_STATUS.enabled}
              onCheckedChange={() => setPending(channel)}
            />
          </li>
        ))}
      </ul>
    )
  }

  return (
    <div className='space-y-3 px-3 pb-4'>
      {/* The controls stay mounted while the query is pending, otherwise the
          search input would unmount after the first keystroke and lose focus. */}
      <div className='space-y-1'>
        <Label htmlFor='mobile-channel-search'>{t('Search channels')}</Label>
        <Input
          id='mobile-channel-search'
          value={filters.name}
          onChange={(event) =>
            setFilters({ ...filters, name: event.target.value })
          }
        />
      </div>

      <div
        role='group'
        aria-label={t('Status')}
        className='flex flex-wrap gap-2'
      >
        {STATUS_FILTERS.map((option) => (
          <button
            key={option.value}
            type='button'
            aria-pressed={filters.statusFilter === option.value}
            onClick={() =>
              setFilters({ ...filters, statusFilter: option.value })
            }
            className={
              filters.statusFilter === option.value
                ? 'text-foreground text-sm font-medium'
                : 'text-muted-foreground text-sm'
            }
          >
            {t(option.label)}
          </button>
        ))}
      </div>

      {content}

      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPending(null)
          }
        }}
        title={enabling ? t('Enable channel') : t('Disable channel')}
        desc={
          enabling
            ? t('Enable this channel?')
            : t('Disable this channel? Traffic will stop routing to it.')
        }
        confirmText={enabling ? t('Enable channel') : t('Disable channel')}
        handleConfirm={confirmToggle}
      />
    </div>
  )
}
