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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export interface CustomRatioRow {
  key: string
  label: string
  /** Editable percentage, only ever seeded from a trusted share. */
  value: string
}

export interface CustomRatioTier {
  priority: number
  rows: CustomRatioRow[]
  /** A tier without a trusted share keeps its current weights untouched. */
  locked: boolean
}

interface CustomRatioDialogProps {
  open: boolean
  tiers: CustomRatioTier[]
  error: string | null
  isSaving: boolean
  onValueChange: (key: string, value: string) => void
  onCancel: () => void
  onConfirm: () => void
}

/**
 * Percentage inputs per priority tier for one model. The page owns the values
 * so cancelling or a failed write never changes the live routing config.
 */
export function CustomRatioDialog(props: CustomRatioDialogProps) {
  const { t } = useTranslation()
  const allTiersLocked =
    props.tiers.length > 0 && props.tiers.every((tier) => tier.locked)

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) props.onCancel()
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('Custom ratio')}</DialogTitle>
          <DialogDescription>
            {t(
              'Set the ratio as percentages. They are stored as integer weights.'
            )}
          </DialogDescription>
        </DialogHeader>
        <div className='space-y-4'>
          {props.tiers.map((tier) => (
            <div key={tier.priority} className='space-y-2'>
              <p className='text-muted-foreground text-xs font-medium'>
                {t('Priority')} {tier.priority}
              </p>
              {tier.locked ? (
                <p className='text-muted-foreground text-xs'>
                  {t(
                    'Channel data is unavailable, so this tier keeps its current weights.'
                  )}
                </p>
              ) : null}
              {tier.rows.map((row) => (
                <div
                  key={row.key}
                  className='flex items-center justify-between gap-3'
                >
                  <Label
                    htmlFor={`routing-ratio-${row.key}`}
                    className='min-w-0 truncate'
                  >
                    {row.label}
                  </Label>
                  <Input
                    id={`routing-ratio-${row.key}`}
                    type='number'
                    min={0}
                    inputMode='decimal'
                    className='w-24'
                    value={row.value}
                    disabled={tier.locked}
                    onChange={
                      tier.locked
                        ? undefined
                        : (event) =>
                            props.onValueChange(row.key, event.target.value)
                    }
                  />
                </div>
              ))}
            </div>
          ))}
        </div>
        {props.error !== null ? (
          <p role='alert' className='text-destructive text-xs'>
            {props.error}
          </p>
        ) : null}
        <DialogFooter>
          <Button
            variant='outline'
            onClick={props.onCancel}
            disabled={props.isSaving}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={props.onConfirm}
            disabled={props.isSaving || allTiersLocked}
          >
            {t('Apply')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
