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
import { useQuery } from '@tanstack/react-query'
import { Plus, Save, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { getChannels } from '@/features/channels/api'

import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

const RANKINGS_CHANNEL_GROUPS_KEY = 'rankings_channel_group_setting.groups'
const CHANNEL_PAGE_SIZE = 100
const MAX_GROUPS = 50
const MAX_GROUP_NAME_LENGTH = 50

type ChannelGroupRow = {
  /** Client-side stable identity for React keys; stripped before saving. */
  key: string
  name: string
  channel_ids: number[]
}

let groupRowKeySeq = 0

const nextGroupRowKey = () => `group-${++groupRowKeySeq}`

async function fetchAllChannels() {
  const firstResponse = await getChannels({
    p: 1,
    page_size: CHANNEL_PAGE_SIZE,
    id_sort: true,
    sort_by: 'id',
    sort_order: 'asc',
  })
  if (!firstResponse.success || !firstResponse.data) {
    throw new Error(firstResponse.message || 'Failed to load channels')
  }

  const totalPages = Math.ceil(
    firstResponse.data.total /
      Math.max(firstResponse.data.page_size, CHANNEL_PAGE_SIZE)
  )
  const remainingResponses = await Promise.all(
    Array.from({ length: Math.max(0, totalPages - 1) }, (_, index) =>
      getChannels({
        p: index + 2,
        page_size: CHANNEL_PAGE_SIZE,
        id_sort: true,
        sort_by: 'id',
        sort_order: 'asc',
      })
    )
  )
  for (const response of remainingResponses) {
    if (!response.success || !response.data) {
      throw new Error(response.message || 'Failed to load channels')
    }
  }

  return [firstResponse, ...remainingResponses].flatMap(
    (response) => response.data?.items ?? []
  )
}

function parseChannelGroups(data: string): ChannelGroupRow[] {
  try {
    const parsed: unknown = JSON.parse(data || '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.map((item) => {
      const record = (item ?? {}) as Record<string, unknown>
      return {
        key: nextGroupRowKey(),
        name: typeof record.name === 'string' ? record.name : '',
        channel_ids: Array.isArray(record.channel_ids)
          ? record.channel_ids
              .map(Number)
              .filter((id) => Number.isInteger(id) && id > 0)
          : [],
      }
    })
  } catch {
    return []
  }
}

type RankingsChannelGroupsSectionProps = {
  data: string
}

export function RankingsChannelGroupsSection({
  data,
}: RankingsChannelGroupsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [groups, setGroups] = useState<ChannelGroupRow[]>([])
  const [hasChanges, setHasChanges] = useState(false)

  useEffect(() => {
    setGroups(parseChannelGroups(data))
    setHasChanges(false)
  }, [data])

  const channelsQuery = useQuery({
    queryKey: ['system-settings', 'rankings-channel-groups', 'channels'],
    queryFn: fetchAllChannels,
    retry: false,
  })

  // Known channels plus fallback entries so IDs that no longer resolve
  // (e.g. deleted channels) still render as selectable chips.
  const channelOptions = useMemo(() => {
    const options = new Map(
      (channelsQuery.data ?? []).map((channel) => [
        channel.id,
        {
          label: `#${channel.id} - ${channel.name}`,
          value: String(channel.id),
        },
      ])
    )
    for (const group of groups) {
      for (const channelId of group.channel_ids) {
        if (!options.has(channelId)) {
          options.set(channelId, {
            label: `#${channelId}`,
            value: String(channelId),
          })
        }
      }
    }
    return [...options.values()].sort(
      (left, right) => Number(left.value) - Number(right.value)
    )
  }, [channelsQuery.data, groups])

  const validationError = useMemo(() => {
    if (groups.length === 0) return null
    const seenNames = new Set<string>()
    for (const group of groups) {
      const name = group.name.trim()
      if (name === '') return t('Group name is required')
      if (name.length > MAX_GROUP_NAME_LENGTH) {
        return t('Group name must be less than 50 characters')
      }
      if (seenNames.has(name)) return t('Group names must be unique')
      seenNames.add(name)
      if (group.channel_ids.length === 0) {
        return t('Each group needs at least one channel')
      }
    }
    return null
  }, [groups, t])

  const updateGroup = (index: number, changes: Partial<ChannelGroupRow>) => {
    setGroups((prev) =>
      prev.map((group, i) => (i === index ? { ...group, ...changes } : group))
    )
    setHasChanges(true)
  }

  const handleAddGroup = () => {
    if (groups.length >= MAX_GROUPS) return
    setGroups((prev) => [
      ...prev,
      { key: nextGroupRowKey(), name: '', channel_ids: [] },
    ])
    setHasChanges(true)
  }

  const handleRemoveGroup = (index: number) => {
    setGroups((prev) => prev.filter((_, i) => i !== index))
    setHasChanges(true)
  }

  const handleSaveAll = async () => {
    if (validationError) return
    try {
      await updateOption.mutateAsync({
        key: RANKINGS_CHANNEL_GROUPS_KEY,
        value: JSON.stringify(
          groups.map((group) => ({
            name: group.name.trim(),
            channel_ids: group.channel_ids,
          }))
        ),
      })
      setHasChanges(false)
      toast.success(t('Rankings channel groups saved successfully'))
    } catch {
      toast.error(t('Failed to save rankings channel groups'))
    }
  }

  return (
    <SettingsSection title={t('Channel aggregation groups')}>
      <p className='text-muted-foreground text-sm'>
        {t(
          'Assign channel IDs to named groups to aggregate their traffic on the rankings page.'
        )}
      </p>

      <div className='flex flex-wrap items-center gap-2'>
        <Button
          onClick={handleAddGroup}
          size='sm'
          disabled={groups.length >= MAX_GROUPS}
        >
          <Plus className='mr-2 h-4 w-4' />
          {t('Add Group')}
        </Button>
        <Button
          onClick={() => void handleSaveAll()}
          size='sm'
          variant='secondary'
          disabled={
            !hasChanges || Boolean(validationError) || updateOption.isPending
          }
        >
          <Save className='mr-2 h-4 w-4' />
          {updateOption.isPending ? t('Saving...') : t('Save Settings')}
        </Button>
        {validationError && (
          <span className='text-destructive text-sm'>{validationError}</span>
        )}
      </div>

      {groups.length === 0 ? (
        <div className='text-muted-foreground/80 rounded-lg border border-dashed px-5 py-8 text-center text-sm'>
          {t('No channel groups yet. Click "Add Group" to create one.')}
        </div>
      ) : (
        <div className='space-y-3'>
          {groups.map((group, index) => (
            <div
              key={group.key}
              className='grid gap-3 rounded-lg border p-3 md:grid-cols-[minmax(0,14rem)_minmax(0,1fr)_auto]'
            >
              <label className='grid gap-1.5 text-sm'>
                <span className='text-muted-foreground text-xs font-medium'>
                  {t('Group name')}
                </span>
                <Input
                  value={group.name}
                  maxLength={MAX_GROUP_NAME_LENGTH}
                  onChange={(event) =>
                    updateGroup(index, { name: event.target.value })
                  }
                  placeholder={t('e.g., Relay A')}
                />
              </label>
              <label className='grid gap-1.5 text-sm'>
                <span className='text-muted-foreground text-xs font-medium'>
                  {t('Channels')}
                </span>
                <MultiSelect
                  options={channelOptions}
                  selected={group.channel_ids.map(String)}
                  onChange={(values) =>
                    updateGroup(index, {
                      channel_ids: values
                        .map(Number)
                        .filter((id) => Number.isInteger(id) && id > 0),
                    })
                  }
                  placeholder={t('Select channels...')}
                  emptyText={
                    channelsQuery.isError
                      ? t('Failed to load channels')
                      : t('No channels found')
                  }
                  maxVisibleChips={12}
                />
              </label>
              <div className='flex items-end justify-end'>
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  onClick={() => handleRemoveGroup(index)}
                  aria-label={t('Remove')}
                >
                  <Trash2 className='h-4 w-4' />
                </Button>
              </div>
            </div>
          ))}
        </div>
      )}
    </SettingsSection>
  )
}
