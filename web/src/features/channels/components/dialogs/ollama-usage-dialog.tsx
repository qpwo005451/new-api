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
import { Check, ChevronDown, ChevronUp, Copy, RefreshCw } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import {
  formatNumber,
  formatTimestampToDate,
  formatTokens,
} from '@/lib/format'
import { cn } from '@/lib/utils'

type OllamaUsageModelEntry = {
  name?: string
  request_count?: number
}

type OllamaUsageWindowData = {
  usage?: number
  models?: OllamaUsageModelEntry[]
}

type OllamaUsageActivity = {
  cost?: string
  period?: {
    type?: string
    starting_at?: string
    ending_at?: string
  }
  models?: unknown[]
}

type OllamaLocalModelUsage = {
  model_name?: string
  requests?: number
  prompt_tokens?: number
  completion_tokens?: number
  level?: number
}

type OllamaLocalUsageWindow = {
  window_seconds?: number
  since?: number
  models?: OllamaLocalModelUsage[]
  total_tokens?: number
  weighted_usage?: number
}

type OllamaUsagePayload = {
  channel_id?: number
  fetched_at?: number
  upstream?: {
    session?: OllamaUsageWindowData
    weekly?: OllamaUsageWindowData
    activity?: OllamaUsageActivity
  }
  local?: {
    session?: OllamaLocalUsageWindow
    weekly?: OllamaLocalUsageWindow
  }
}

export type OllamaUsageDialogData = {
  success: boolean
  message?: string
  data?: Record<string, unknown>
}

type OllamaUsageDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  channelName?: string
  channelId?: number
  channelDisplayName?: string
  channelDisplayId?: string
  response: OllamaUsageDialogData | null
  onRefresh?: () => void | Promise<void>
  isRefreshing?: boolean
}

// Ollama usage levels from the model pages (Low = 1 ... Extra High = 4);
// level 0 means the model is not in the known-levels table.
const OLLAMA_LEVEL_BADGE: Record<
  number,
  { label: string; variant: StatusBadgeProps['variant'] }
> = {
  1: { label: 'Low', variant: 'success' },
  2: { label: 'Medium', variant: 'info' },
  3: { label: 'High', variant: 'warning' },
  4: { label: 'Extra High', variant: 'danger' },
}

function getLevelBadge(
  level: unknown,
  t: (key: string) => string
): { label: string; variant: StatusBadgeProps['variant'] } {
  const key = Number(level)
  const known = OLLAMA_LEVEL_BADGE[key]
  if (known) {
    return { label: t(known.label), variant: known.variant }
  }
  return { label: t('Unknown'), variant: 'neutral' as const }
}

function formatUsageNumber(value: unknown): string {
  const v = Number(value)
  return Number.isFinite(v) ? formatNumber(v) : '-'
}

function sumUpstreamRequests(
  window?: OllamaUsageWindowData
): number | undefined {
  const models = window?.models
  if (!models || models.length === 0) {
    return undefined
  }
  return models.reduce((sum, m) => sum + Number(m.request_count ?? 0), 0)
}

function UpstreamUsageCard(props: {
  title: string
  window?: OllamaUsageWindowData
}) {
  const { t } = useTranslation()
  const models = props.window?.models ?? []

  return (
    <Card size='sm' className='gap-0 py-0'>
      <CardHeader className='p-3 pb-2'>
        <div className='flex items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardTitle className='text-sm font-semibold'>
              {props.title}
            </CardTitle>
            <CardDescription className='mt-1 text-xs'>
              {t('Reported by Ollama')}
            </CardDescription>
          </div>
          <div className='shrink-0 text-right'>
            <div className='text-xl leading-none font-semibold tabular-nums'>
              {formatUsageNumber(props.window?.usage)}
            </div>
            <div className='text-muted-foreground mt-1 text-[11px]'>
              {t('Usage units')}
            </div>
          </div>
        </div>
      </CardHeader>
      <CardContent className='p-3 pt-0'>
        {models.length > 0 ? (
          <div className='mt-1 flex flex-col gap-1'>
            {models.map((model) => (
              <div
                key={model.name ?? ''}
                className='flex items-start justify-between gap-2 text-xs'
              >
                <span className='min-w-0 break-all font-mono'>
                  {model.name || '-'}
                </span>
                <span className='text-muted-foreground shrink-0 tabular-nums'>
                  {t('{{count}} requests', { count: model.request_count ?? 0 })}
                </span>
              </div>
            ))}
          </div>
        ) : (
          <div className='text-muted-foreground mt-1 text-xs'>
            {t('No requests in this window')}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

function LocalModelRow(props: { model: OllamaLocalModelUsage }) {
  const { t } = useTranslation()
  const level = getLevelBadge(props.model.level, t)
  const promptTokens = Number(props.model.prompt_tokens ?? 0)
  const completionTokens = Number(props.model.completion_tokens ?? 0)

  return (
    <div className='bg-background ring-border/60 flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg px-2 py-1.5 ring-1'>
      <span className='min-w-0 flex-1 break-all font-mono text-xs'>
        {props.model.model_name || '-'}
      </span>
      <span className='text-muted-foreground shrink-0 text-[11px] tabular-nums'>
        {t('{{count}} requests', { count: props.model.requests ?? 0 })}
      </span>
      <span className='shrink-0 text-xs tabular-nums'>
        {t('Prompt {{value}} / Completion {{value2}}', {
          value: formatTokens(promptTokens),
          value2: formatTokens(completionTokens),
        })}
      </span>
      <StatusBadge
        label={level.label}
        variant={level.variant}
        copyable={false}
      />
    </div>
  )
}

function LocalUsageCard(props: {
  title: string
  window?: OllamaLocalUsageWindow
  upstreamRequests?: number
}) {
  const { t } = useTranslation()
  const models = props.window?.models ?? []
  const localRequests = models.reduce((s, m) => s + Number(m.requests ?? 0), 0)

  return (
    <Card size='sm' className='gap-0 py-0'>
      <CardHeader className='p-3 pb-2'>
        <div className='flex items-start justify-between gap-3'>
          <div className='min-w-0'>
            <CardTitle className='text-sm font-semibold'>
              {props.title}
            </CardTitle>
            <CardDescription className='mt-1 text-xs'>
              {t("Estimated from this channel's request logs")}
            </CardDescription>
          </div>
          <div className='shrink-0 text-right'>
            <div className='text-xl leading-none font-semibold tabular-nums'>
              {formatUsageNumber(props.window?.weighted_usage)}
            </div>
            <div className='text-muted-foreground mt-1 text-[11px]'>
              {t('Weighted tokens')}
            </div>
          </div>
        </div>
      </CardHeader>
      <CardContent className='p-3 pt-0'>
        <div className='text-muted-foreground text-xs tabular-nums'>
          <span>{t('Total tokens:')}</span>{' '}
          <span className='text-foreground'>
            {formatTokens(Number(props.window?.total_tokens ?? 0))}
          </span>
          {' · '}
          <span>{t('Requests:')}</span>{' '}
          <span className='text-foreground'>{localRequests}</span>
          {props.upstreamRequests !== undefined ? (
            <>
              {' · '}
              <span>{t('Upstream requests:')}</span>{' '}
              <span className='text-foreground'>{props.upstreamRequests}</span>
            </>
          ) : null}
        </div>
        {models.length > 0 ? (
          <div className='mt-2 flex flex-col gap-1'>
            {models.map((model) => (
              <LocalModelRow
                key={
                  model.model_name ||
                  `${model.requests ?? 0}-${model.prompt_tokens ?? 0}-${model.completion_tokens ?? 0}`
                }
                model={model}
              />
            ))}
          </div>
        ) : (
          <div className='text-muted-foreground mt-2 text-xs'>
            {t('No requests in this window')}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export function OllamaUsageDialog(props: OllamaUsageDialogProps) {
  const { t } = useTranslation()
  const { copiedText, copyToClipboard } = useCopyToClipboard({ notify: false })
  const [showRawJson, setShowRawJson] = useState(false)

  const payload: OllamaUsagePayload | null = useMemo(() => {
    const raw = props.response?.data
    if (!raw || typeof raw !== 'object') {
      return null
    }
    return raw as OllamaUsagePayload
  }, [props.response?.data])

  const channelLabelName = props.channelDisplayName ?? props.channelName ?? '-'
  let channelLabelId = ''
  if (props.channelDisplayId != null) {
    channelLabelId = ` (#${props.channelDisplayId})`
  } else if (props.channelId) {
    channelLabelId = ` (#${props.channelId})`
  }
  const channelLabel = `${channelLabelName}${channelLabelId}`
  const errorMessage =
    props.response?.success === false
      ? props.response?.message?.trim() || t('Failed to fetch usage')
      : ''

  const rawJsonText = useMemo(() => {
    if (!props.response) {
      return ''
    }
    try {
      return JSON.stringify(props.response, null, 2)
    } catch {
      return String(props.response?.data ?? '')
    }
  }, [props.response])

  const handleDialogOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      setShowRawJson(false)
    }
    props.onOpenChange(nextOpen)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={handleDialogOpenChange}
      title={t('Ollama Usage')}
      contentClassName='sm:max-w-[900px]'
      contentHeight='auto'
      bodyClassName='flex flex-col gap-4'
      footer={
        <Button
          type='button'
          variant='outline'
          onClick={() => handleDialogOpenChange(false)}
        >
          {t('Close')}
        </Button>
      }
    >
      <div className='flex flex-col gap-4'>
        {errorMessage && (
          <div className='rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-800 dark:bg-red-950/30 dark:text-red-400'>
            {errorMessage}
          </div>
        )}

        {payload ? (
          <>
            <div className='flex flex-wrap items-center justify-between gap-2 text-xs'>
              <div className='flex flex-wrap items-center gap-2'>
                <StatusBadge
                  label={channelLabel}
                  variant='neutral'
                  copyable={false}
                />
                {payload.upstream?.activity?.cost !== undefined ? (
                  <StatusBadge
                    label={`${t('4-week activity cost:')} ${payload.upstream.activity.cost}`}
                    variant='blue'
                    copyable={false}
                  />
                ) : null}
              </div>
              <div className='text-muted-foreground flex items-center gap-2'>
                <span className='tabular-nums'>
                  {t('Fetched at:')}{' '}
                  {payload.fetched_at
                    ? formatTimestampToDate(payload.fetched_at)
                    : '-'}
                </span>
                {props.onRefresh ? (
                  <Button
                    type='button'
                    variant='outline'
                    size='icon-xs'
                    aria-label={t('Refresh')}
                    onClick={props.onRefresh}
                    disabled={Boolean(props.isRefreshing)}
                  >
                    <RefreshCw
                      className={cn(props.isRefreshing && 'animate-spin')}
                    />
                  </Button>
                ) : null}
              </div>
            </div>

            <div className='flex flex-col gap-3'>
              <div className='text-sm font-semibold'>{t('Upstream Usage')}</div>
              <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
                <UpstreamUsageCard
                  title={t('5-Hour Window')}
                  window={payload.upstream?.session}
                />
                <UpstreamUsageCard
                  title={t('Weekly Window')}
                  window={payload.upstream?.weekly}
                />
              </div>
            </div>

            <div className='flex flex-col gap-3'>
              <div className='text-sm font-semibold'>{t('Local Estimate')}</div>
              <div className='grid grid-cols-1 gap-3 md:grid-cols-2'>
                <LocalUsageCard
                  title={t('5-Hour Window')}
                  window={payload.local?.session}
                  upstreamRequests={sumUpstreamRequests(
                    payload.upstream?.session
                  )}
                />
                <LocalUsageCard
                  title={t('Weekly Window')}
                  window={payload.local?.weekly}
                  upstreamRequests={sumUpstreamRequests(
                    payload.upstream?.weekly
                  )}
                />
              </div>
              <div className='text-muted-foreground text-xs leading-5'>
                {t(
                  'Weighted tokens estimate upstream usage units as model level × (prompt + completion) tokens, because Ollama does not publish the cap or the unit. Upstream and local request counts are directly comparable; mismatches mean some upstream traffic in this window predates the current channel or key.'
                )}
              </div>
            </div>
          </>
        ) : null}

        <Collapsible
          open={showRawJson}
          onOpenChange={setShowRawJson}
          className='rounded-lg border'
        >
          <CollapsibleTrigger
            render={
              <button
                type='button'
                className='hover:bg-muted/40 flex w-full items-center justify-between gap-2 p-3 transition-colors'
                aria-expanded={showRawJson}
              />
            }
          >
            <div className='text-sm font-medium'>{t('Raw JSON')}</div>
            {showRawJson ? (
              <ChevronUp className='text-muted-foreground h-4 w-4' />
            ) : (
              <ChevronDown className='text-muted-foreground h-4 w-4' />
            )}
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className='flex justify-end border-t px-3 py-2'>
              <Button
                type='button'
                variant='outline'
                size='sm'
                onClick={() => copyToClipboard(rawJsonText)}
                disabled={!rawJsonText}
              >
                {copiedText === rawJsonText ? (
                  <Check data-icon='inline-start' className='text-success' />
                ) : (
                  <Copy data-icon='inline-start' />
                )}
                {t('Copy')}
              </Button>
            </div>
            <ScrollArea className='max-h-[50vh]'>
              <pre className='bg-muted/30 m-0 p-3 text-xs break-words whitespace-pre-wrap'>
                {rawJsonText || '-'}
              </pre>
            </ScrollArea>
          </CollapsibleContent>
        </Collapsible>
      </div>
    </Dialog>
  )
}
