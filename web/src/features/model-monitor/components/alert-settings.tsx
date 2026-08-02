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

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Bell, Mail, Plus, Save, Send, Trash2 } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

import {
  getModelMonitorAlertConfig,
  testModelMonitorAlerts,
  updateModelMonitorAlertConfig,
} from '../api'
import type {
  ModelMonitorAlertConfig,
  ModelMonitorAlertRule,
  ModelMonitorSiteConfig,
} from '../types'

type AlertSettingsProps = {
  sites: ModelMonitorSiteConfig[]
}

function createRule(sites: ModelMonitorSiteConfig[]): ModelMonitorAlertRule {
  const site = sites.find((item) => item.id != null)
  return {
    site_id: site?.id ?? 0,
    channel_id: site?.channel_ids[0] ?? 0,
    model_prefix: '',
    model_name: '',
    enabled: true,
  }
}

export function AlertSettings(props: AlertSettingsProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ModelMonitorAlertConfig | null>(null)
  const alertQuery = useQuery({
    queryKey: ['model-monitor', 'alerts'],
    queryFn: getModelMonitorAlertConfig,
    retry: false,
  })

  useEffect(() => {
    if (alertQuery.data?.success && alertQuery.data.data) {
      setDraft(alertQuery.data.data)
    }
  }, [alertQuery.data])

  const saveMutation = useMutation({
    mutationFn: updateModelMonitorAlertConfig,
    onSuccess: async (response) => {
      if (!response.success || !response.data) {
        toast.error(response.message || t('Failed to save alert settings'))
        return
      }
      setDraft(response.data)
      await queryClient.invalidateQueries({
        queryKey: ['model-monitor', 'alerts'],
      })
      toast.success(t('Alert settings saved'))
    },
  })
  const testMutation = useMutation({
    mutationFn: testModelMonitorAlerts,
    onSuccess: (response) => {
      if (!response.success) {
        toast.error(response.message || t('Alert test failed'))
        return
      }
      toast.success(t('Test notifications sent'))
    },
  })

  if (alertQuery.isLoading || !draft) {
    return (
      <Card>
        <CardContent className='text-muted-foreground py-6 text-sm'>
          {t('Loading...')}
        </CardContent>
      </Card>
    )
  }

  const replaceRule = (index: number, rule: ModelMonitorAlertRule) => {
    const rules = [...draft.rules]
    rules[index] = rule
    setDraft({ ...draft, rules })
  }

  return (
    <Card>
      <CardHeader>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div>
            <CardTitle className='flex items-center gap-2'>
              <Bell className='size-4' aria-hidden='true' />
              {t('Availability alerts')}
            </CardTitle>
            <CardDescription>
              {t(
                'Notify selected transports when a monitored model becomes unavailable or recovers.'
              )}
            </CardDescription>
          </div>
          <div className='flex gap-2'>
            <Button
              type='button'
              variant='outline'
              onClick={() => testMutation.mutate()}
              disabled={!draft.enabled || testMutation.isPending}
            >
              <Send data-icon='inline-start' aria-hidden='true' />
              {testMutation.isPending
                ? t('Sending...')
                : t('Send test notification')}
            </Button>
            <Button
              type='button'
              onClick={() => saveMutation.mutate(draft)}
              disabled={saveMutation.isPending}
            >
              <Save data-icon='inline-start' aria-hidden='true' />
              {saveMutation.isPending ? t('Saving...') : t('Save alerts')}
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent className='space-y-4'>
        <label className='flex items-center justify-between gap-3 rounded-md border p-3'>
          <span>
            <span className='block font-medium'>{t('Alerts enabled')}</span>
            <span className='text-muted-foreground text-xs'>
              {t('Only unavailable and recovery transitions send alerts.')}
            </span>
          </span>
          <Switch
            checked={draft.enabled}
            onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
          />
        </label>

        <div className='grid gap-3 lg:grid-cols-2'>
          <div className='space-y-3 rounded-md border p-3'>
            <label className='flex items-center justify-between gap-3'>
              <span className='flex items-center gap-2 font-medium'>
                <Mail className='size-4' aria-hidden='true' />
                {t('Email notifications')}
              </span>
              <Switch
                checked={draft.email_enabled}
                onCheckedChange={(email_enabled) =>
                  setDraft({ ...draft, email_enabled })
                }
              />
            </label>
            <label className='grid gap-1.5 text-sm'>
              <span className='font-medium'>{t('Recipients')}</span>
              <Input
                value={draft.email_recipients}
                placeholder='ops@example.com; owner@example.com'
                disabled={!draft.email_enabled}
                onChange={(event) =>
                  setDraft({
                    ...draft,
                    email_recipients: event.target.value,
                  })
                }
              />
            </label>
          </div>

          <div className='space-y-3 rounded-md border p-3'>
            <label className='flex items-center justify-between gap-3'>
              <span className='flex items-center gap-2 font-medium'>
                <Send className='size-4' aria-hidden='true' />
                {t('Telegram notifications')}
              </span>
              <Switch
                checked={draft.telegram_enabled}
                onCheckedChange={(telegram_enabled) =>
                  setDraft({ ...draft, telegram_enabled })
                }
              />
            </label>
            <div className='grid gap-3 sm:grid-cols-2'>
              <label className='grid gap-1.5 text-sm'>
                <span className='font-medium'>{t('Bot token')}</span>
                <Input
                  type='password'
                  value={draft.telegram_bot_token ?? ''}
                  placeholder={
                    draft.telegram_bot_token_configured
                      ? t('Token configured; leave blank to keep it')
                      : t('Telegram bot token')
                  }
                  disabled={!draft.telegram_enabled}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      telegram_bot_token: event.target.value,
                    })
                  }
                />
              </label>
              <label className='grid gap-1.5 text-sm'>
                <span className='font-medium'>{t('Chat ID')}</span>
                <Input
                  value={draft.telegram_chat_id}
                  disabled={!draft.telegram_enabled}
                  onChange={(event) =>
                    setDraft({
                      ...draft,
                      telegram_chat_id: event.target.value,
                    })
                  }
                />
              </label>
            </div>
          </div>
        </div>

        <div className='space-y-3'>
          <div className='flex items-center justify-between gap-3'>
            <div>
              <h3 className='font-medium'>{t('Alert rules')}</h3>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Each rule selects one site, channel, and either an exact model or model prefix.'
                )}
              </p>
            </div>
            <Button
              type='button'
              variant='outline'
              onClick={() =>
                setDraft({
                  ...draft,
                  rules: [...draft.rules, createRule(props.sites)],
                })
              }
            >
              <Plus data-icon='inline-start' aria-hidden='true' />
              {t('Add rule')}
            </Button>
          </div>

          <div className='space-y-2'>
            {draft.rules.map((rule, index) => {
              const site = props.sites.find((item) => item.id === rule.site_id)
              const selectorType = rule.model_name ? 'exact' : 'prefix'
              const selectorValue =
                selectorType === 'exact'
                  ? (rule.model_name ?? '')
                  : (rule.model_prefix ?? '')
              return (
                <div
                  key={`${rule.site_id}-${rule.channel_id}-${rule.model_name ?? ''}-${rule.model_prefix ?? ''}`}
                  className='grid gap-2 rounded-md border p-3 md:grid-cols-[auto_minmax(10rem,1fr)_minmax(8rem,0.7fr)_8rem_minmax(12rem,1.2fr)_auto] md:items-end'
                >
                  <label className='flex h-8 items-center gap-2 text-sm'>
                    <Switch
                      checked={rule.enabled}
                      onCheckedChange={(enabled) =>
                        replaceRule(index, { ...rule, enabled })
                      }
                    />
                    {t('Enabled')}
                  </label>
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>{t('Site')}</span>
                    <Select
                      value={String(rule.site_id || '')}
                      onValueChange={(value) => {
                        const nextSite = props.sites.find(
                          (item) => String(item.id) === value
                        )
                        if (!nextSite?.id) return
                        replaceRule(index, {
                          ...rule,
                          site_id: nextSite.id,
                          channel_id: nextSite.channel_ids[0] ?? 0,
                        })
                      }}
                    >
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {props.sites
                          .filter((item) => item.id != null)
                          .map((item) => (
                            <SelectItem key={item.id} value={String(item.id)}>
                              {item.name}
                            </SelectItem>
                          ))}
                      </SelectContent>
                    </Select>
                  </label>
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>{t('Channel')}</span>
                    <Select
                      value={String(rule.channel_id || '')}
                      onValueChange={(value) =>
                        replaceRule(index, {
                          ...rule,
                          channel_id: Number(value),
                        })
                      }
                    >
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {(site?.channel_ids ?? []).map((channelId) => (
                          <SelectItem key={channelId} value={String(channelId)}>
                            #{channelId}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </label>
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>{t('Match')}</span>
                    <Select
                      value={selectorType}
                      onValueChange={(value) => {
                        if (value === 'exact') {
                          replaceRule(index, {
                            ...rule,
                            model_name: selectorValue,
                            model_prefix: '',
                          })
                        } else if (value === 'prefix') {
                          replaceRule(index, {
                            ...rule,
                            model_name: '',
                            model_prefix: selectorValue,
                          })
                        }
                      }}
                    >
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='prefix'>
                          {t('Model prefix')}
                        </SelectItem>
                        <SelectItem value='exact'>
                          {t('Exact model')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </label>
                  <label className='grid gap-1.5 text-sm'>
                    <span className='font-medium'>
                      {selectorType === 'exact'
                        ? t('Exact model')
                        : t('Model prefix')}
                    </span>
                    <Input
                      value={selectorValue}
                      placeholder={
                        selectorType === 'exact' ? 'kimi-k2.7-code' : 'gpt-'
                      }
                      onChange={(event) =>
                        replaceRule(index, {
                          ...rule,
                          model_name:
                            selectorType === 'exact' ? event.target.value : '',
                          model_prefix:
                            selectorType === 'prefix' ? event.target.value : '',
                        })
                      }
                    />
                  </label>
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    aria-label={t('Delete rule')}
                    onClick={() =>
                      setDraft({
                        ...draft,
                        rules: draft.rules.filter(
                          (_, ruleIndex) => ruleIndex !== index
                        ),
                      })
                    }
                  >
                    <Trash2 aria-hidden='true' />
                  </Button>
                </div>
              )
            })}
          </div>
        </div>
      </CardContent>
    </Card>
  )
}
