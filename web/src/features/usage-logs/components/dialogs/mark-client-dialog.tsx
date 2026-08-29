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
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { useUsageLogsContext } from '../usage-logs-provider'

// MarkClientDialog lets admins attach a display name to an unrecognized
// User-Agent so the usage log views can label it from then on.
export function MarkClientDialog() {
  const { t } = useTranslation()
  const {
    clientAliases,
    saveClientAlias,
    markClientDialogUa,
    setMarkClientDialogUa,
  } = useUsageLogsContext()
  const [name, setName] = useState('')
  const [saving, setSaving] = useState(false)

  const open = markClientDialogUa != null
  const existingAlias =
    markClientDialogUa != null ? clientAliases[markClientDialogUa] : undefined

  useEffect(() => {
    if (open) {
      setName(existingAlias ?? '')
    }
    // Re-sync the input only when the dialog opens for a (new) user agent;
    // live alias updates should not clobber in-progress edits.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, markClientDialogUa])

  const handleSave = async () => {
    if (!markClientDialogUa) return
    setSaving(true)
    try {
      const res = await saveClientAlias(markClientDialogUa, name)
      if (res.success) {
        toast.success(
          name.trim() ? t('Client label saved.') : t('Client label removed.')
        )
        setMarkClientDialogUa(null)
      } else {
        toast.error(res.message || t('Failed to save client label.'))
      }
    } catch {
      toast.error(t('Failed to save client label.'))
    } finally {
      setSaving(false)
    }
  }

  const handleRemove = async () => {
    if (!markClientDialogUa) return
    setSaving(true)
    try {
      const res = await saveClientAlias(markClientDialogUa, '')
      if (res.success) {
        toast.success(t('Client label removed.'))
        setMarkClientDialogUa(null)
      } else {
        toast.error(res.message || t('Failed to save client label.'))
      }
    } catch {
      toast.error(t('Failed to save client label.'))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setMarkClientDialogUa(null)
      }}
      title={t('Mark Client')}
      description={t(
        'Label this client so future requests are easier to recognize.'
      )}
      contentClassName='sm:max-w-md'
      contentHeight='auto'
      footer={
        <>
          {existingAlias && (
            <Button
              type='button'
              variant='destructive'
              disabled={saving}
              onClick={handleRemove}
            >
              {t('Remove')}
            </Button>
          )}
          <div className='flex-1' />
          <Button
            type='button'
            variant='outline'
            disabled={saving}
            onClick={() => setMarkClientDialogUa(null)}
          >
            {t('Cancel')}
          </Button>
          <Button type='button' disabled={saving} onClick={handleSave}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-2'>
        <div className='space-y-1.5'>
          <Label className='text-muted-foreground text-xs'>
            {t('User Agent')}
          </Label>
          <div className='bg-muted/30 rounded-md px-2 py-1.5 font-mono text-xs leading-relaxed break-all'>
            {markClientDialogUa}
          </div>
        </div>
        <div className='space-y-1.5'>
          <Label
            className='text-muted-foreground text-xs'
            htmlFor='client-alias-name'
          >
            {t('Display Name')}
          </Label>
          <Input
            id='client-alias-name'
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t('Custom client name')}
            maxLength={60}
          />
        </div>
      </div>
    </Dialog>
  )
}
