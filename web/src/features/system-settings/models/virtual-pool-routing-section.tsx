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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { JsonCodeEditor } from '@/components/json-code-editor'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { formatJsonForTextarea, normalizeJsonString } from './utils'

const capacityFieldsSchema = z.object({
  capacity: z.number().int().min(1).max(100000).optional(),
  weight: z.number().positive().max(1000).optional(),
  shared_capacity_group: z.string().trim().max(128).optional(),
})

const virtualModelRouteSchema = z.object({
  rotation: z.enum(['ordered', 'random', 'round_robin']).optional(),
  max_attempts: z.number().int().min(0).max(100).optional(),
  capacity_groups: z
    .record(
      z.string().trim().min(1).max(128),
      z.object({
        capacity: z.number().int().min(1).max(100000),
      })
    )
    .optional(),
  health: z
    .object({
      enabled: z.boolean(),
      failure_threshold: z.number().int().min(1).optional(),
      cooldown_seconds: z.number().int().min(0).optional(),
      max_cooldown_seconds: z.number().int().min(0).optional(),
    })
    .optional(),
  targets: z
    .array(
      capacityFieldsSchema.extend({
        model: z.string().trim().min(1),
        channel_id: z.number().int().positive().optional(),
      })
    )
    .optional(),
  sources: z
    .array(capacityFieldsSchema.extend({ channel_id: z.number().int().positive() }))
    .optional(),
})

const routesSchema = z.record(z.string().trim().min(1), virtualModelRouteSchema)

const createVirtualPoolRoutingSchema = (t: (key: string) => string) =>
  z.object({
    routes: z.string().superRefine((value, ctx) => {
      try {
        routesSchema.parse(JSON.parse(value))
      } catch {
        ctx.addIssue({
          code: 'custom',
          message: t('Enter a valid virtual model routes JSON object'),
        })
      }
    }),
  })

type VirtualPoolRoutingFormValues = z.output<
  ReturnType<typeof createVirtualPoolRoutingSchema>
>
type VirtualPoolRoutingFormInput = z.input<
  ReturnType<typeof createVirtualPoolRoutingSchema>
>

type Props = {
  defaultValues: {
    'model_retry_policy_setting.virtual_model_routes': string
  }
}

const virtualPoolRoutesExample = JSON.stringify(
  {
    'deepseek-v4.1-flash': {
      rotation: 'round_robin',
      max_attempts: 3,
      capacity_groups: {
        'input-subscriptions': { capacity: 15 },
        ollama: { capacity: 3 },
      },
      targets: [
        {
          model: 'deepseek-v4.1-flash',
          weight: 1,
          shared_capacity_group: 'input-subscriptions',
        },
        {
          model: 'deepseek-v4.1-flash',
          weight: 1,
          shared_capacity_group: 'ollama',
        },
      ],
    },
  },
  null,
  2
)

export function VirtualPoolRoutingSection(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = createVirtualPoolRoutingSchema(t)
  const normalizedDefault = formatJsonForTextarea(
    props.defaultValues['model_retry_policy_setting.virtual_model_routes']
  )
  const form = useForm<
    VirtualPoolRoutingFormInput,
    unknown,
    VirtualPoolRoutingFormValues
  >({
    resolver: zodResolver(schema),
    defaultValues: { routes: normalizedDefault },
  })

  useEffect(() => {
    form.reset({ routes: normalizedDefault })
  }, [form, normalizedDefault])

  const onSubmit = async (values: VirtualPoolRoutingFormValues) => {
    const normalized = normalizeJsonString(values.routes)
    if (normalized === normalizeJsonString(normalizedDefault)) {
      toast.info(t('No changes to save'))
      return
    }
    await updateOption.mutateAsync({
      key: 'model_retry_policy_setting.virtual_model_routes',
      value: normalized,
    })
  }

  const formatJson = () => {
    try {
      form.setValue('routes', JSON.stringify(JSON.parse(form.getValues('routes')), null, 2), {
        shouldDirty: true,
        shouldValidate: true,
      })
    } catch {
      toast.error(t('Invalid JSON format'))
    }
  }

  return (
    <SettingsSection title={t('Virtual Pool Routing')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />
          <FormField
            control={form.control}
            name='routes'
            render={({ field }) => (
              <FormItem className='flex min-w-0 flex-col gap-2'>
                <FormLabel>{t('Virtual model routes')}</FormLabel>
                <FormControl>
                  <JsonCodeEditor
                    value={field.value}
                    onChange={field.onChange}
                    name={field.name}
                    onBlur={field.onBlur}
                    textareaRef={field.ref}
                    heightClassName='h-96 min-h-96 max-h-96'
                    placeholder={virtualPoolRoutesExample}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Capacity limits concurrent upstream attempts. Sources or targets sharing the same shared_capacity_group share one limit, such as 15 for three subscriptions or 3 for Ollama.'
                  )}
                </FormDescription>
                <div className='flex flex-wrap gap-2'>
                  <Button type='button' variant='outline' onClick={formatJson}>
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
