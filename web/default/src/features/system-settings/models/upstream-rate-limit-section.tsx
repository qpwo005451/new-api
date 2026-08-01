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
import { zodResolver } from '@hookform/resolvers/zod'
import { useEffect, useMemo, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Button } from '@/components/design-system/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const upstreamRateLimitRuleSchema = z.object({
  name: z.string().trim().min(1),
  base_url_host: z.string().trim().min(1),
  models: z.array(z.string().trim().min(1)).min(1),
  rpm: z.number().int().min(1).max(600),
  cooldown_seconds: z.number().int().min(0).max(3600),
})

const createUpstreamRateLimitSchema = (t: (key: string) => string) =>
  z.object({
    enabled: z.boolean(),
    rules: z.string().superRefine((value, ctx) => {
      try {
        const parsed = JSON.parse(value)
        z.array(upstreamRateLimitRuleSchema).parse(parsed)
      } catch {
        ctx.addIssue({
          code: 'custom',
          message: t('Enter a valid upstream RPM rules JSON array'),
        })
      }
    }),
  })

type UpstreamRateLimitFormValues = z.output<
  ReturnType<typeof createUpstreamRateLimitSchema>
>
type UpstreamRateLimitFormInput = z.input<
  ReturnType<typeof createUpstreamRateLimitSchema>
>

type Props = {
  defaultValues: {
    'upstream_rate_limit_setting.enabled': boolean
    'upstream_rate_limit_setting.rules': string
  }
}

function formatRules(value: string) {
  try {
    return JSON.stringify(JSON.parse(value), null, 2)
  } catch {
    return '[]'
  }
}

function normalizeRules(value: string) {
  return JSON.stringify(JSON.parse(value))
}

const upstreamRateLimitExample = JSON.stringify(
  [
    {
      name: 'Input Kimi K2.7 Code',
      base_url_host: 'ai.input.im',
      models: ['kimi-k2.7-code'],
      rpm: 10,
      cooldown_seconds: 60,
    },
  ],
  null,
  2
)

export function UpstreamRateLimitSection(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const upstreamRateLimitSchema = createUpstreamRateLimitSchema(t)
  const normalizedDefaults = useMemo(
    () => ({
      enabled:
        props.defaultValues['upstream_rate_limit_setting.enabled'] ?? false,
      rules: formatRules(
        props.defaultValues['upstream_rate_limit_setting.rules']
      ),
    }),
    [props.defaultValues]
  )
  const baselineRef = useRef(normalizedDefaults)
  const form = useForm<
    UpstreamRateLimitFormInput,
    unknown,
    UpstreamRateLimitFormValues
  >({
    resolver: zodResolver(upstreamRateLimitSchema),
    defaultValues: normalizedDefaults,
  })

  useResetForm(form, normalizedDefaults)
  useEffect(() => {
    baselineRef.current = normalizedDefaults
  }, [normalizedDefaults])

  const formatRulesField = () => {
    try {
      const formatted = JSON.stringify(
        JSON.parse(form.getValues('rules')),
        null,
        2
      )
      form.setValue('rules', formatted, {
        shouldDirty: true,
        shouldValidate: true,
      })
    } catch {
      toast.error(t('Invalid JSON format'))
    }
  }

  const onSubmit = async (values: UpstreamRateLimitFormValues) => {
    const normalized = {
      enabled: values.enabled,
      rules: normalizeRules(values.rules),
    }
    const updates = [
      ['upstream_rate_limit_setting.enabled', normalized.enabled],
      ['upstream_rate_limit_setting.rules', normalized.rules],
    ] as const

    const changed = updates.filter(([key, value]) => {
      if (key === 'upstream_rate_limit_setting.enabled') {
        return value !== baselineRef.current.enabled
      }
      return value !== normalizeRules(baselineRef.current.rules)
    })
    if (changed.length === 0) {
      toast.info(t('No changes to save'))
      return
    }
    for (const [key, value] of changed) {
      await updateOption.mutateAsync({ key, value })
    }
    baselineRef.current = {
      enabled: normalized.enabled,
      rules: formatRules(normalized.rules),
    }
  }

  return (
    <SettingsSection title={t('Upstream Request Limits')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable upstream RPM limits')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Queue matching requests before the upstream call so rate-limit responses do not reach clients.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='rules'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Upstream RPM rules')}</FormLabel>
                <FormControl>
                  <Textarea
                    className='min-h-80 font-mono text-xs'
                    placeholder={`${t('Example:')}\n${upstreamRateLimitExample}`}
                    spellCheck={false}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Each rule matches a base URL host and models. Limits are applied separately for each selected upstream key and model. A 429 applies the configured cooldown before the next attempt.'
                  )}
                </FormDescription>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    onClick={formatRulesField}
                  >
                    {t('Format JSON')}
                  </Button>
                </div>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
