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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import { TitledCard } from '@/components/ui/titled-card'
import { formatNumber, formatPercent, formatUseTime } from '@/lib/format'

import {
  actualShares,
  buildLiveAllocation,
  configuredShares,
  routingChannelKey,
  routingChannelLabel,
} from '../lib'
import type {
  ChannelAffinityBinding,
  RoutingAffinityStat,
  RoutingModelChannelStat,
  RoutingSwitchStat,
} from '../types'

export function DistributionTable(props: { rows: RoutingModelChannelStat[] }) {
  const { t } = useTranslation()

  const columns = useMemo(() => {
    const actual = actualShares(props.rows)
    const configured = configuredShares(props.rows)
    return [
      {
        id: 'model',
        header: t('Model'),
        cell: (row: RoutingModelChannelStat) => (
          <span className='font-medium'>{row.model_name}</span>
        ),
      },
      {
        id: 'channel',
        header: t('Channel'),
        cell: (row: RoutingModelChannelStat) => (
          <span>
            {routingChannelLabel(row.channel_id, row.channel_name)}
            <span className='text-muted-foreground ml-1 text-xs'>
              #{row.channel_id}
            </span>
          </span>
        ),
      },
      {
        id: 'requests',
        header: t('Requests'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: RoutingModelChannelStat) => formatNumber(row.requests),
      },
      {
        id: 'actual',
        header: t('Actual share'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: RoutingModelChannelStat) =>
          formatPercent(
            (actual.get(routingChannelKey(row.model_name, row.channel_id)) ??
              0) * 100
          ),
      },
      {
        id: 'configured',
        header: t('Configured share'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: RoutingModelChannelStat) => {
          const share = configured.get(
            routingChannelKey(row.model_name, row.channel_id)
          )
          if (share === undefined) return '-'
          return (
            <span>
              {formatPercent(share * 100)}
              <span className='text-muted-foreground ml-1 text-xs'>
                p{row.priority ?? '-'} / w{row.weight ?? '-'}
              </span>
            </span>
          )
        },
      },
      {
        id: 'errors',
        header: t('Errors'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: RoutingModelChannelStat) => formatNumber(row.errors),
      },
      {
        id: 'latency',
        header: t('Avg. time'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: RoutingModelChannelStat) => formatUseTime(row.avg_use_time),
      },
    ]
  }, [props.rows, t])

  return (
    <TitledCard
      title={t('Channel distribution')}
      description={t(
        'Observed split beside the priority and weight that produced it.'
      )}
    >
      <StaticDataTable
        data={props.rows}
        columns={columns}
        getRowKey={(row) => routingChannelKey(row.model_name, row.channel_id)}
        emptyContent={t('No routing data in the selected window.')}
      />
    </TitledCard>
  )
}

export function SwitchFlowTable(props: {
  switches: RoutingSwitchStat[]
  channelNames: Map<number, string>
}) {
  const { t } = useTranslation()

  const columns = useMemo(
    () => [
      {
        id: 'from',
        header: t('From'),
        cell: (row: RoutingSwitchStat) => (
          <span>
            {routingChannelLabel(
              row.from,
              props.channelNames.get(row.from) ?? ''
            )}
            <span className='text-muted-foreground ml-1 text-xs'>
              #{row.from}
            </span>
          </span>
        ),
      },
      {
        id: 'to',
        header: t('To'),
        cell: (row: RoutingSwitchStat) => (
          <span>
            {routingChannelLabel(row.to, props.channelNames.get(row.to) ?? '')}
            <span className='text-muted-foreground ml-1 text-xs'>
              #{row.to}
            </span>
          </span>
        ),
      },
      {
        id: 'count',
        header: t('Switches'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: RoutingSwitchStat) => formatNumber(row.count),
      },
    ],
    [props.channelNames, t]
  )

  return (
    <TitledCard
      title={t('Channel switches')}
      description={t('Which channel a request moved away from and to.')}
    >
      <StaticDataTable
        data={props.switches}
        columns={columns}
        getRowKey={(row) => `${row.from}-${row.to}`}
        emptyContent={t('No channel switch in the selected window.')}
      />
    </TitledCard>
  )
}

export function AffinityTable(props: { rules: RoutingAffinityStat[] }) {
  const { t } = useTranslation()

  const columns = useMemo(
    () => [
      {
        id: 'rule',
        header: t('Rule'),
        cell: (row: RoutingAffinityStat) => row.rule_name,
      },
      {
        id: 'sticky',
        header: t('Sticky requests'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: RoutingAffinityStat) => formatNumber(row.sticky_requests),
      },
      {
        id: 'sessions',
        header: t('Distinct sessions'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: RoutingAffinityStat) => formatNumber(row.distinct_keys),
      },
    ],
    [t]
  )

  return (
    <TitledCard
      title={t('Session stickiness')}
      description={t('Affinity rules that pinned requests to a channel.')}
    >
      <StaticDataTable
        data={props.rules}
        columns={columns}
        getRowKey={(row) => row.rule_name}
        emptyContent={t('No sticky requests in the selected window.')}
      />
    </TitledCard>
  )
}

export function LiveAllocationTable(props: {
  entries: ChannelAffinityBinding[]
}) {
  const { t } = useTranslation()

  const rows = useMemo(
    () => buildLiveAllocation(props.entries),
    [props.entries]
  )
  const shares = useMemo(
    () =>
      actualShares(
        rows.map((row) => ({
          model_name: row.modelName,
          channel_id: row.channelID,
          requests: row.sessions,
        }))
      ),
    [rows]
  )

  const columns = useMemo(
    () => [
      {
        id: 'model',
        header: t('Model'),
        cell: (row: (typeof rows)[number]) => (
          <span className='font-medium'>{row.modelName}</span>
        ),
      },
      {
        id: 'channel',
        header: t('Channel'),
        cell: (row: (typeof rows)[number]) => (
          <span>
            {routingChannelLabel(row.channelID, row.channelName)}
            <span className='text-muted-foreground ml-1 text-xs'>
              #{row.channelID}
            </span>
          </span>
        ),
      },
      {
        id: 'sessions',
        header: t('Sessions'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: (typeof rows)[number]) => formatNumber(row.sessions),
      },
      {
        id: 'share',
        header: t('Actual share'),
        className: 'text-right',
        cellClassName: 'text-right',
        cell: (row: (typeof rows)[number]) =>
          formatPercent(
            (shares.get(routingChannelKey(row.modelName, row.channelID)) ?? 0) *
              100
          ),
      },
    ],
    [shares, t]
  )

  return (
    <TitledCard
      title={t('Current session allocation')}
      description={t(
        'Sessions held right now per model and channel. This is current state, not a time window.'
      )}
    >
      <StaticDataTable
        data={rows}
        columns={columns}
        getRowKey={(row) => routingChannelKey(row.modelName, row.channelID)}
        emptyContent={t('No session is pinned to a channel right now.')}
      />
    </TitledCard>
  )
}

export function LiveBindingsTable(props: {
  entries: ChannelAffinityBinding[]
}) {
  const { t } = useTranslation()

  const columns = useMemo(
    () => [
      {
        id: 'model',
        header: t('Model'),
        cell: (row: ChannelAffinityBinding) => (
          <span className='font-medium'>{row.model_name}</span>
        ),
      },
      {
        id: 'channel',
        header: t('Channel'),
        cell: (row: ChannelAffinityBinding) => (
          <span>
            {routingChannelLabel(row.channel_id, row.channel_name)}
            <span className='text-muted-foreground ml-1 text-xs'>
              #{row.channel_id}
            </span>
          </span>
        ),
      },
      {
        id: 'group',
        header: t('Group'),
        cell: (row: ChannelAffinityBinding) => row.using_group || '-',
      },
      {
        id: 'rule',
        header: t('Rule'),
        cell: (row: ChannelAffinityBinding) => row.rule_name,
      },
      {
        id: 'session',
        header: t('Session'),
        cell: (row: ChannelAffinityBinding) => (
          <span className='font-mono text-xs'>{row.key_hint}</span>
        ),
      },
    ],
    [t]
  )

  return (
    <TitledCard
      title={t('Pinned sessions')}
      description={t('Every session that is currently bound to a channel.')}
    >
      <StaticDataTable
        data={props.entries}
        columns={columns}
        getRowKey={(row) =>
          `${row.rule_name}-${row.model_name}-${row.using_group}-${row.key_fingerprint}`
        }
        emptyContent={t('No session is pinned to a channel right now.')}
      />
    </TitledCard>
  )
}
