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

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { entryKey, tierTotal, type CustomRatioState } from './model-weight-rows'

type ModelWeightCustomRatioDialogProps = {
  state: CustomRatioState | null
  isPending: boolean
  onOpenChange: (open: boolean) => void
  onPercentChange: (key: string, value: string) => void
  onConfirm: () => void
}

export function ModelWeightCustomRatioDialog(
  props: ModelWeightCustomRatioDialogProps
) {
  const { t } = useTranslation()
  const state = props.state
  const hasPositiveShare = state
    ? Object.values(state.percents).some((value) => Number(value) > 0)
    : false

  return (
    <Dialog
      open={state !== null}
      onOpenChange={props.onOpenChange}
      title={t('Custom ratio')}
      description={t(
        'Set the ratio as percentages. They are stored as integer weights.'
      )}
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='button'
            disabled={!hasPositiveShare || props.isPending}
            onClick={props.onConfirm}
          >
            {t('Apply')}
          </Button>
        </>
      }
    >
      {state ? (
        <div className='space-y-4'>
          {state.tiers.length === 0 ? (
            <p className='text-muted-foreground text-sm'>
              {t('No overrides yet. Click "Add override" to create one.')}
            </p>
          ) : (
            state.tiers.map((tier) => (
              <div key={tier.tier} className='space-y-2 rounded-lg border p-3'>
                <div className='flex items-center justify-between gap-2 text-xs'>
                  <span className='text-muted-foreground font-medium'>
                    {t('Priority')} {tier.tier}
                  </span>
                  <span className='text-muted-foreground'>
                    {t('Total')}: {tierTotal(tier, state.percents)}%
                  </span>
                </div>
                {tier.entries.map((entry) => (
                  <label
                    key={entryKey(entry)}
                    className='flex items-center justify-between gap-3 text-sm'
                  >
                    <span>#{entry.channel_id}</span>
                    <Input
                      aria-label={`${t('Share')} #${entry.channel_id}`}
                      type='number'
                      min={0}
                      max={100}
                      value={state.percents[entryKey(entry)] ?? ''}
                      onChange={(event) =>
                        props.onPercentChange(
                          entryKey(entry),
                          event.target.value
                        )
                      }
                    />
                  </label>
                ))}
              </div>
            ))
          )}
          {!hasPositiveShare ? (
            <p className='text-destructive text-sm'>
              {t('Enter a ratio greater than 0.')}
            </p>
          ) : null}
        </div>
      ) : null}
    </Dialog>
  )
}
