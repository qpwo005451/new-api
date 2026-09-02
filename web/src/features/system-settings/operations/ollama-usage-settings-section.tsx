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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { removeTrailingSlash } from '../integrations/utils'

const createOllamaUsageSchema = (t: (key: string) => string) =>
  z.object({
    OllamaUsageWebhookUrl: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^https?:\/\//.test(trimmed)
    }, t('Provide a valid URL starting with http:// or https://')),
    OllamaUsageWebhookSecret: z.string(),
  })

type OllamaUsageFormValues = z.infer<ReturnType<typeof createOllamaUsageSchema>>

type OllamaUsageSettingsSectionProps = {
  defaultValues: OllamaUsageFormValues
}

export function OllamaUsageSettingsSection({
  defaultValues,
}: OllamaUsageSettingsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const ollamaUsageSchema = createOllamaUsageSchema(t)

  const form = useForm<OllamaUsageFormValues>({
    resolver: zodResolver(ollamaUsageSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (values: OllamaUsageFormValues) => {
    const sanitizedUrl = removeTrailingSlash(values.OllamaUsageWebhookUrl)
    const sanitizedSecret = values.OllamaUsageWebhookSecret.trim()
    const initialUrl = removeTrailingSlash(defaultValues.OllamaUsageWebhookUrl)
    const initialSecret = defaultValues.OllamaUsageWebhookSecret.trim()

    const updates: Array<{ key: string; value: string }> = []

    if (sanitizedUrl !== initialUrl) {
      updates.push({ key: 'OllamaUsageWebhookUrl', value: sanitizedUrl })
    }

    // The secret is masked in the settings response, so the initial value is
    // always empty; a non-empty input replaces it, and clearing the URL also
    // clears the secret.
    if (sanitizedSecret !== initialSecret || sanitizedUrl === '') {
      updates.push({ key: 'OllamaUsageWebhookSecret', value: sanitizedSecret })
    }

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection title={t('Ollama Usage Monitor')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save Ollama usage monitor settings'
          />
          <FormField
            control={form.control}
            name='OllamaUsageWebhookUrl'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Ollama usage refresh webhook URL')}</FormLabel>
                <FormControl>
                  <Input
                    type='url'
                    inputMode='url'
                    placeholder={t(
                      'https://n8n.example.com/webhook/ollama-usage-refresh'
                    )}
                    autoComplete='off'
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'POSTed to trigger an immediate ollama.com usage scrape; the response carries the authoritative used percent and window reset times shown in the Ollama usage panel. Leave blank to keep using the local estimate.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='OllamaUsageWebhookSecret'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {t('Ollama usage refresh webhook secret')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={t('Enter new secret to update')}
                    autoComplete='new-password'
                    {...field}
                    onChange={(event) => field.onChange(event.target.value)}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Sent as the X-Ollama-Refresh-Secret header. Must match the value configured in the N8N webhook credential. Leave blank to keep the existing secret.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}
