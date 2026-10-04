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
import { Plus, Settings2, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

import { useSavePolicy } from '../request-policies/use-save-policy'
import {
  findActivePresetName,
  MODEL_WEIGHTS_OPTION_KEY,
  MODEL_WEIGHT_PRESETS_OPTION_KEY,
  parseModelWeightEntries,
  serializeModelWeightPresets,
  serializePresetWeights,
  type ModelWeightPreset,
} from './model-weight-presets'

const PRESET_NAME_MAX_LENGTH = 64

type Props = {
  presets: ModelWeightPreset[]
  /** Serialized current editor rows; drives the active highlight and Save-as-preset. */
  currentWeights: string
}

export function ModelWeightPresetBar({ presets, currentWeights }: Props) {
  const { t } = useTranslation()
  const savePolicy = useSavePolicy()
  const [pendingApply, setPendingApply] = useState<ModelWeightPreset | null>(
    null
  )
  const [pendingDelete, setPendingDelete] = useState<string | null>(null)
  const [manageOpen, setManageOpen] = useState(false)
  const [presetName, setPresetName] = useState('')

  const activeName = findActivePresetName(presets, currentWeights)

  const handleApply = async () => {
    if (!pendingApply) return
    await savePolicy.mutateAsync({
      [MODEL_WEIGHTS_OPTION_KEY]: serializePresetWeights(pendingApply),
    })
    setPendingApply(null)
  }

  const handleSavePreset = async () => {
    const name = presetName.trim()
    if (name === '') {
      toast.error(t('Enter a preset name'))
      return
    }
    const next = [
      ...presets.filter(
        (preset) => preset.name.toLowerCase() !== name.toLowerCase()
      ),
      { name, weights: parseModelWeightEntries(currentWeights) },
    ]
    await savePolicy.mutateAsync({
      [MODEL_WEIGHT_PRESETS_OPTION_KEY]: serializeModelWeightPresets(next),
    })
    setPresetName('')
    setManageOpen(false)
  }

  const handleDelete = async () => {
    if (pendingDelete === null) return
    const next = presets.filter((preset) => preset.name !== pendingDelete)
    await savePolicy.mutateAsync({
      [MODEL_WEIGHT_PRESETS_OPTION_KEY]: serializeModelWeightPresets(next),
    })
    setPendingDelete(null)
  }

  return (
    <div className='bg-muted/30 space-y-2 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center gap-2'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t('Quick switch')}
        </span>
        {presets.length === 0 ? (
          <span className='text-muted-foreground text-xs'>
            {t('No presets yet.')}
          </span>
        ) : (
          presets.map((preset) => (
            <Button
              key={preset.name}
              type='button'
              size='sm'
              variant={preset.name === activeName ? 'default' : 'outline'}
              aria-pressed={preset.name === activeName}
              onClick={() => setPendingApply(preset)}
            >
              {preset.name}
            </Button>
          ))
        )}
        {presets.length > 0 && activeName === null ? (
          <span className='text-muted-foreground text-xs'>
            {t('Custom routing')}
          </span>
        ) : null}
        <Button
          type='button'
          size='sm'
          variant='ghost'
          onClick={() => setManageOpen(true)}
        >
          <Settings2 data-icon='inline-start' />
          {t('Manage presets')}
        </Button>
      </div>

      <ConfirmDialog
        open={pendingApply !== null}
        onOpenChange={(open) => {
          if (!open) setPendingApply(null)
        }}
        title={t('Apply preset')}
        desc={
          <span>
            {t('Apply preset {{name}}?', { name: pendingApply?.name ?? '' })}{' '}
            {t('This changes live routing immediately.')}
          </span>
        }
        confirmText={t('Apply')}
        isLoading={savePolicy.isPending}
        handleConfirm={() => void handleApply()}
      />

      <Dialog
        open={manageOpen}
        onOpenChange={setManageOpen}
        title={t('Presets')}
        description={t(
          'Save the current overrides as a preset and apply it later with one click.'
        )}
        footer={
          <Button
            type='button'
            variant='outline'
            onClick={() => setManageOpen(false)}
          >
            {t('Close')}
          </Button>
        }
      >
        <div className='space-y-4'>
          {presets.length === 0 ? (
            <p className='text-muted-foreground text-sm'>
              {t('No presets yet.')}
            </p>
          ) : (
            <div className='space-y-2'>
              {presets.map((preset) => (
                <div
                  key={preset.name}
                  className='flex items-center justify-between gap-2 rounded-lg border p-2'
                >
                  <span className='min-w-0 truncate text-sm'>
                    {preset.name}
                  </span>
                  <Button
                    type='button'
                    variant='outline'
                    size='icon'
                    aria-label={t('Remove')}
                    onClick={() => setPendingDelete(preset.name)}
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))}
            </div>
          )}

          <div className='flex items-end gap-2'>
            <label className='grid flex-1 gap-1.5 text-sm'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Preset name')}
              </span>
              <Input
                aria-label={t('Preset name')}
                value={presetName}
                maxLength={PRESET_NAME_MAX_LENGTH}
                onChange={(event) => setPresetName(event.target.value)}
              />
            </label>
            <Button
              type='button'
              disabled={savePolicy.isPending}
              onClick={() => void handleSavePreset()}
            >
              <Plus data-icon='inline-start' />
              {t('Save current as preset')}
            </Button>
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null)
        }}
        title={t('Delete preset')}
        desc={t('Delete preset {{name}}?', { name: pendingDelete ?? '' })}
        destructive
        confirmText={t('Delete')}
        isLoading={savePolicy.isPending}
        handleConfirm={() => void handleDelete()}
      />
    </div>
  )
}
