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
import { useQueryClient } from '@tanstack/react-query'
import { Loader2, RefreshCw, ShieldCheck } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import type { UseFormReturn } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/design-system/button'
import { Input } from '@/components/design-system/input'
import {
  SideDrawerSection,
  SideDrawerSectionHeader,
} from '@/components/drawer-layout'
import { MultiSelect } from '@/components/multi-select'
import { StatusBadge, type StatusBadgeProps } from '@/components/status-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { toIntlLocale } from '@/i18n/languages'

import { fetchUpstreamModels, updateChannelBalance } from '../../../api'
import {
  channelsQueryKeys,
  formatModelsArray,
  parseModelsString,
} from '../../../lib'
import type { ChannelFormValues } from '../../../lib/channel-form'

type ChannelBalanceProtectionSectionProps = {
  channelId: number
  form: UseFormReturn<ChannelFormValues>
  isMultiKey: boolean
  supported: boolean
}

const STATE_PRESENTATION: Record<
  ChannelFormValues['balance_protection']['state'],
  { label: string; variant: StatusBadgeProps['variant'] }
> = {
  disabled: { label: 'Disabled', variant: 'neutral' },
  pending: { label: 'Pending verification', variant: 'warning' },
  normal: { label: 'Full routing', variant: 'success' },
  protected: { label: 'Free models only', variant: 'warning' },
  unknown: { label: 'Balance unknown', variant: 'destructive' },
  invalid_allowlist: {
    label: 'Free model list invalid',
    variant: 'destructive',
  },
}

function formatTimestamp(timestamp: number, locale: string): string {
  if (!Number.isFinite(timestamp) || timestamp <= 0) return '-'
  return new Date(timestamp * 1000).toLocaleString(toIntlLocale(locale))
}

export function ChannelBalanceProtectionSection(
  props: ChannelBalanceProtectionSectionProps
) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [discoveredModels, setDiscoveredModels] = useState<string[]>([])
  const [isFetchingModels, setIsFetchingModels] = useState(false)
  const [isCheckingBalance, setIsCheckingBalance] = useState(false)

  const protection = props.form.watch('balance_protection')
  const currentModels = props.form.watch('models')
  const currentModelsArray = useMemo(
    () => parseModelsString(currentModels),
    [currentModels]
  )
  const modelOptions = useMemo(() => {
    const models = new Set([
      ...currentModelsArray,
      ...discoveredModels,
      ...protection.free_models,
    ])
    return [...models]
      .sort((a, b) => a.localeCompare(b))
      .map((model) => ({
        label: model,
        value: model,
      }))
  }, [currentModelsArray, discoveredModels, protection.free_models])

  const statePresentation = STATE_PRESENTATION[protection.state]
  const cannotEnable = !props.supported || props.isMultiKey
  const protectionDirty = Boolean(
    props.form.formState.dirtyFields.balance_protection
  )

  useEffect(() => {
    setDiscoveredModels([])
  }, [props.channelId])

  const handleFetchModels = async () => {
    setIsFetchingModels(true)
    try {
      const response = await fetchUpstreamModels(props.channelId)
      if (!response.success) {
        throw new Error(response.message || t('Failed to fetch models'))
      }
      const models = Array.isArray(response.data) ? response.data : []
      setDiscoveredModels(models)
      toast.success(t('Fetched {{count}} models', { count: models.length }))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to fetch models')
      )
    } finally {
      setIsFetchingModels(false)
    }
  }

  const handleCheckBalance = async () => {
    setIsCheckingBalance(true)
    try {
      const response = await updateChannelBalance(props.channelId)
      if (!response.success || !response.balance_protection) {
        throw new Error(response.message || t('Failed to query balance'))
      }
      props.form.setValue('balance_protection', response.balance_protection, {
        shouldDirty: false,
      })
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: channelsQueryKeys.lists() }),
        queryClient.invalidateQueries({
          queryKey: channelsQueryKeys.detail(props.channelId),
        }),
      ])
      toast.success(t('Balance queried successfully'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Failed to query balance')
      )
    } finally {
      setIsCheckingBalance(false)
    }
  }

  const handleFreeModelsChange = (models: string[]) => {
    props.form.setValue('balance_protection.free_models', models, {
      shouldDirty: true,
      shouldValidate: true,
    })
    const mergedModels = formatModelsArray([...currentModelsArray, ...models])
    props.form.setValue('models', mergedModels, {
      shouldDirty: true,
      shouldValidate: true,
    })
  }

  return (
    <SideDrawerSection>
      <SideDrawerSectionHeader
        title={t('Balance Protection')}
        description={t(
          'Keep selected free models available when the upstream balance is low.'
        )}
        icon={<ShieldCheck className='h-4 w-4' aria-hidden='true' />}
      />

      {!props.supported && (
        <Alert>
          <AlertDescription>
            {t('This channel type does not support balance protection.')}
          </AlertDescription>
        </Alert>
      )}
      {props.isMultiKey && (
        <Alert>
          <AlertDescription>
            {t('Multi-key channels do not support balance protection.')}
          </AlertDescription>
        </Alert>
      )}

      <div className='divide-border divide-y border-y'>
        <FormField
          control={props.form.control}
          name='balance_protection.enabled'
          render={({ field }) => (
            <FormItem className='flex items-center justify-between gap-4 px-4 py-3'>
              <div className='space-y-0.5'>
                <FormLabel>{t('Enable balance protection')}</FormLabel>
                <FormDescription>
                  {t(
                    'Paid models fall through to other channels while protection is active.'
                  )}
                </FormDescription>
              </div>
              <FormControl>
                <Switch
                  checked={field.value}
                  disabled={cannotEnable && !field.value}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
            </FormItem>
          )}
        />
        <FormField
          control={props.form.control}
          name='balance_protection.notify_enabled'
          render={({ field }) => (
            <FormItem className='flex items-center justify-between gap-4 px-4 py-3'>
              <div className='space-y-0.5'>
                <FormLabel>{t('Notify on state changes')}</FormLabel>
                <FormDescription>
                  {t('Send one administrator notification per transition.')}
                </FormDescription>
              </div>
              <FormControl>
                <Switch
                  checked={field.value}
                  disabled={!protection.enabled}
                  onCheckedChange={field.onChange}
                />
              </FormControl>
            </FormItem>
          )}
        />
      </div>

      <div className='grid gap-4 sm:grid-cols-3'>
        <FormField
          control={props.form.control}
          name='balance_protection.trigger_balance'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Protection threshold (USD)')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={0}
                  step='0.01'
                  disabled={!protection.enabled}
                  value={field.value}
                  onChange={(event) =>
                    field.onChange(Number(event.target.value))
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={props.form.control}
          name='balance_protection.recovery_balance'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Recovery threshold (USD)')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={0}
                  step='0.01'
                  disabled={!protection.enabled}
                  value={field.value}
                  onChange={(event) =>
                    field.onChange(Number(event.target.value))
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={props.form.control}
          name='balance_protection.check_interval_minutes'
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t('Check interval (minutes)')}</FormLabel>
              <FormControl>
                <Input
                  type='number'
                  min={1}
                  max={60}
                  step={1}
                  disabled={!protection.enabled}
                  value={field.value}
                  onChange={(event) =>
                    field.onChange(Number(event.target.value))
                  }
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>

      <FormField
        control={props.form.control}
        name='balance_protection.free_models'
        render={({ field }) => (
          <FormItem>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <FormLabel>{t('Free models')}</FormLabel>
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={!protection.enabled || isFetchingModels}
                onClick={handleFetchModels}
              >
                {isFetchingModels ? (
                  <Loader2 className='size-4 animate-spin' aria-hidden='true' />
                ) : (
                  <RefreshCw className='size-4' aria-hidden='true' />
                )}
                {t('Refresh upstream models')}
              </Button>
            </div>
            <FormControl>
              <MultiSelect
                options={modelOptions}
                selected={field.value}
                onChange={handleFreeModelsChange}
                placeholder={t('Select free models')}
                emptyText={t(
                  'Refresh upstream models to load available models'
                )}
                disabled={!protection.enabled || cannotEnable}
                maxVisibleChips={6}
              />
            </FormControl>
            <FormDescription>
              {t(
                'Exact caller-facing model IDs only. Selected models are also added to the channel model list.'
              )}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <div className='border-t pt-4'>
        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='flex items-center gap-2'>
            <span className='text-sm font-medium'>{t('Runtime status')}</span>
            <StatusBadge variant={statePresentation.variant}>
              {t(statePresentation.label)}
            </StatusBadge>
          </div>
          <Button
            type='button'
            variant='outline'
            size='sm'
            disabled={isCheckingBalance || !props.supported || protectionDirty}
            onClick={handleCheckBalance}
          >
            {isCheckingBalance ? (
              <Loader2 className='size-4 animate-spin' aria-hidden='true' />
            ) : (
              <RefreshCw className='size-4' aria-hidden='true' />
            )}
            {t('Check balance now')}
          </Button>
        </div>
        <dl className='text-muted-foreground mt-3 grid gap-x-6 gap-y-2 text-xs sm:grid-cols-2'>
          <div>
            <dt className='text-foreground inline font-medium'>
              {t('Last check')}:
            </dt>{' '}
            <dd className='inline'>
              {formatTimestamp(
                protection.last_check_time,
                i18n.resolvedLanguage || i18n.language
              )}
            </dd>
          </div>
          <div>
            <dt className='text-foreground inline font-medium'>
              {t('Last successful check')}:
            </dt>{' '}
            <dd className='inline'>
              {formatTimestamp(
                protection.last_success_time,
                i18n.resolvedLanguage || i18n.language
              )}
            </dd>
          </div>
          <div>
            <dt className='text-foreground inline font-medium'>
              {t('Consecutive failures')}:
            </dt>{' '}
            <dd className='inline tabular-nums'>
              {protection.consecutive_failures}
            </dd>
          </div>
          <div>
            <dt className='text-foreground inline font-medium'>
              {t('Last transition')}:
            </dt>{' '}
            <dd className='inline'>
              {formatTimestamp(
                protection.last_transition_time,
                i18n.resolvedLanguage || i18n.language
              )}
            </dd>
          </div>
        </dl>
        {protection.last_error && (
          <p className='text-destructive mt-3 text-xs break-words'>
            {protection.last_error}
          </p>
        )}
      </div>
    </SideDrawerSection>
  )
}
