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
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

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
const GLOBAL_PRESET_VALUE = '__global__'

type Props = {
  presets: ModelWeightPreset[]
  /** Serialized SAVED weights; drives the active highlight. */
  currentWeights: string
  /** Serialized editor rows; drives Save-as-preset and the scope model list. */
  editorWeights: string
}

function scopeKey(model: string | undefined) {
  return (model ?? '').trim().toLowerCase()
}

function presetKey(preset: ModelWeightPreset) {
  return `${scopeKey(preset.model)}|${preset.name.trim().toLowerCase()}`
}

export function ModelWeightPresetBar({
  presets,
  currentWeights,
  editorWeights,
}: Props) {
  const { t } = useTranslation()
  const savePolicy = useSavePolicy()
  const [pendingApply, setPendingApply] = useState<ModelWeightPreset | null>(
    null
  )
  const [pendingDelete, setPendingDelete] = useState<{
    name: string
    scope: string
  } | null>(null)
  const [manageOpen, setManageOpen] = useState(false)
  const [presetName, setPresetName] = useState('')
  const [scope, setScope] = useState(GLOBAL_PRESET_VALUE)

  const globalPresets = useMemo(
    () => presets.filter((preset) => scopeKey(preset.model) === ''),
    [presets]
  )
  const activeName = findActivePresetName(globalPresets, currentWeights)

  const modelOptions = useMemo(() => {
    const seen = new Set<string>()
    const models: string[] = []
    for (const entry of parseModelWeightEntries(editorWeights)) {
      const model = entry.model.trim()
      const key = model.toLowerCase()
      if (model === '' || seen.has(key)) continue
      seen.add(key)
      models.push(model)
    }
    return models
  }, [editorWeights])

  const scopeItems = [
    { label: t('Global (all models)'), value: GLOBAL_PRESET_VALUE },
    ...modelOptions.map((model) => ({
      label: t('Only {{model}}', { model }),
      value: model,
    })),
  ]

  const scopeLabel = (preset: ModelWeightPreset) => {
    const scoped = (preset.model ?? '').trim()
    return scoped === ''
      ? t('Global (all models)')
      : t('Only {{model}}', { model: scoped })
  }

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
    const targetScope = scope === GLOBAL_PRESET_VALUE ? '' : scope
    const allWeights = parseModelWeightEntries(editorWeights)
    const weights =
      targetScope === ''
        ? allWeights
        : allWeights.filter(
            (entry) => scopeKey(entry.model) === scopeKey(targetScope)
          )
    const preset: ModelWeightPreset =
      targetScope === ''
        ? { name, weights }
        : { name, model: targetScope, weights }
    const next = [
      ...presets.filter(
        (existing) =>
          !(
            scopeKey(existing.model) === scopeKey(targetScope) &&
            existing.name.trim().toLowerCase() === name.toLowerCase()
          )
      ),
      preset,
    ]
    await savePolicy.mutateAsync({
      [MODEL_WEIGHT_PRESETS_OPTION_KEY]: serializeModelWeightPresets(next),
    })
    setPresetName('')
    setManageOpen(false)
  }

  const handleDelete = async () => {
    if (pendingDelete === null) return
    const next = presets.filter(
      (preset) =>
        !(
          scopeKey(preset.model) === scopeKey(pendingDelete.scope) &&
          preset.name.trim().toLowerCase() ===
            pendingDelete.name.trim().toLowerCase()
        )
    )
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
        {globalPresets.length === 0 ? (
          <span className='text-muted-foreground text-xs'>
            {t('No presets yet.')}
          </span>
        ) : (
          globalPresets.map((preset) => (
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
        {globalPresets.length > 0 && activeName === null ? (
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
                  key={presetKey(preset)}
                  className='flex items-center justify-between gap-2 rounded-lg border p-2'
                >
                  <div className='flex min-w-0 flex-col'>
                    <span className='truncate text-sm'>{preset.name}</span>
                    <span className='text-muted-foreground text-xs'>
                      {scopeLabel(preset)}
                    </span>
                  </div>
                  <Button
                    type='button'
                    variant='outline'
                    size='icon'
                    aria-label={t('Remove')}
                    onClick={() =>
                      setPendingDelete({
                        name: preset.name,
                        scope: (preset.model ?? '').trim(),
                      })
                    }
                  >
                    <Trash2 />
                  </Button>
                </div>
              ))}
            </div>
          )}

          <div className='flex flex-wrap items-end gap-2'>
            <label className='grid min-w-40 flex-1 gap-1.5 text-sm'>
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
            <label className='grid min-w-40 gap-1.5 text-sm'>
              <span className='text-muted-foreground text-xs font-medium'>
                {t('Preset scope')}
              </span>
              <Select
                items={scopeItems}
                value={scope}
                onValueChange={(value) => value !== null && setScope(value)}
              >
                <SelectTrigger
                  aria-label={t('Preset scope')}
                  className='w-full'
                >
                  <SelectValue />
                </SelectTrigger>
                <SelectContent alignItemWithTrigger={false}>
                  {scopeItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
        desc={t('Delete preset {{name}}?', {
          name: pendingDelete?.name ?? '',
        })}
        destructive
        confirmText={t('Delete')}
        isLoading={savePolicy.isPending}
        handleConfirm={() => void handleDelete()}
      />
    </div>
  )
}
