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
import { Edit, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { StaticDataTable } from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'

import { SettingsSwitchField } from '../components/settings-form-layout'
import { SettingsPageActionsPortal } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'

export type ModelHealthPolicyRule = {
  id?: number
  name: string
  enabled: boolean
  models?: string[]
  model_regex?: string[]
  failure_threshold?: number
  cooldown_seconds?: number
  status_codes?: number[]
  groups?: string[]
}

type Props = {
  defaultValues: {
    'model_health_policy_setting.enabled': boolean
    'model_health_policy_setting.rules': string
  }
}

function parseRules(value: string): ModelHealthPolicyRule[] {
  try {
    const parsed = JSON.parse(value || '{"enabled":false,"rules":[]}')
    const rules = Array.isArray(parsed) ? parsed : parsed.rules
    if (!Array.isArray(rules)) return []
    return rules.map(
      (rule: Record<string, unknown>, index: number) =>
        ({ id: index, enabled: true, ...rule }) as ModelHealthPolicyRule
    )
  } catch {
    return []
  }
}

function serializeRules(rules: ModelHealthPolicyRule[]) {
  return JSON.stringify(rules.map(({ id: _, ...rule }) => rule))
}

function parseList(value: string) {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

export function ModelHealthPolicySection(props: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [enabled, setEnabled] = useState(
    props.defaultValues['model_health_policy_setting.enabled']
  )
  const [rules, setRules] = useState<ModelHealthPolicyRule[]>(() =>
    parseRules(props.defaultValues['model_health_policy_setting.rules'])
  )
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingRule, setEditingRule] = useState<ModelHealthPolicyRule | null>(
    null
  )
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    setEnabled(props.defaultValues['model_health_policy_setting.enabled'])
    setRules(
      parseRules(props.defaultValues['model_health_policy_setting.rules'])
    )
  }, [props.defaultValues])

  const originalRules = useMemo(
    () => props.defaultValues['model_health_policy_setting.rules'],
    [props.defaultValues]
  )

  const save = async () => {
    setSaving(true)
    try {
      const updates: { key: string; value: string }[] = []
      if (enabled !== props.defaultValues['model_health_policy_setting.enabled']) {
        updates.push({
          key: 'model_health_policy_setting.enabled',
          value: String(enabled),
        })
      }
      const serialized = serializeRules(rules)
      if (serialized !== originalRules) {
        updates.push({
          key: 'model_health_policy_setting.rules',
          value: serialized,
        })
      }
      if (updates.length === 0) {
        toast.info(t('No changes'))
        return
      }
      for (const update of updates) {
        await updateOption.mutateAsync(update)
      }
    } catch {
      toast.error(t('Failed to save'))
    } finally {
      setSaving(false)
    }
  }

  const removeRule = (index: number) => {
    setRules((current) =>
      current.filter((_, currentIndex) => currentIndex !== index)
    )
  }

  const saveRule = (rule: ModelHealthPolicyRule) => {
    setRules((current) => {
      const next = [...current]
      if (editingRule?.id !== undefined) {
        next[editingRule.id] = rule
      } else {
        next.push(rule)
      }
      return next.map((item, index) => ({ ...item, id: index }))
    })
    setEditingRule(null)
    setEditorOpen(false)
  }

  return (
    <>
      <SettingsSection title={t('Model Health Policy')}>
        <SettingsSwitchField
          checked={enabled}
          onCheckedChange={setEnabled}
          label={t('Enable model-level channel cooldown')}
          description={t(
            'Tracks upstream failures per model and channel. A cooling channel is skipped for that model only, then retried after the cooldown.'
          )}
        />
        <Separator />
        <SettingsPageActionsPortal>
          <Button
            size='sm'
            variant='outline'
            onClick={() => {
              setEditingRule(null)
              setEditorOpen(true)
            }}
          >
            <Plus className='mr-1 h-3 w-3' />
            {t('Add Rule')}
          </Button>
          <Button size='sm' onClick={save} disabled={saving}>
            {saving ? t('Saving...') : t('Save')}
          </Button>
        </SettingsPageActionsPortal>
        <StaticDataTable
          tableClassName='min-w-max'
          data={rules}
          emptyContent={t('No rules yet')}
          columns={[
            {
              id: 'name',
              header: t('Name'),
              cellClassName: 'font-medium',
              cell: (rule) => rule.name,
            },
            {
              id: 'models',
              header: t('Models'),
              cell: (rule) =>
                [...(rule.models || []), ...(rule.model_regex || [])].join(
                  ', '
                ) || '-',
            },
            {
              id: 'threshold',
              header: t('Failure threshold'),
              cell: (rule) => rule.failure_threshold ?? 2,
            },
            {
              id: 'cooldown',
              header: t('Cooldown seconds'),
              cell: (rule) => rule.cooldown_seconds ?? 300,
            },
            {
              id: 'status',
              header: t('Status'),
              cell: (rule) => (
                <StatusBadge
                  label={rule.enabled === false ? t('Disabled') : t('Enabled')}
                  variant={rule.enabled === false ? 'neutral' : 'success'}
                  copyable={false}
                />
              ),
            },
            {
              id: 'actions',
              header: t('Actions'),
              className: 'text-right',
              cellClassName: 'text-right',
              cell: (rule, index) => (
                <div className='flex justify-end gap-1'>
                  <Button
                    variant='ghost'
                    size='icon'
                    className='h-7 w-7'
                    onClick={() => {
                      setEditingRule({ ...rule, id: index })
                      setEditorOpen(true)
                    }}
                  >
                    <Edit className='h-3 w-3' />
                  </Button>
                  <Button
                    variant='ghost'
                    size='icon'
                    className='h-7 w-7'
                    onClick={() => removeRule(index)}
                  >
                    <Trash2 className='h-3 w-3' />
                  </Button>
                </div>
              ),
            },
          ]}
        />
      </SettingsSection>
      <ModelHealthRuleDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        rule={editingRule}
        onSave={saveRule}
      />
    </>
  )
}

function ModelHealthRuleDialog(props: {
  open: boolean
  onOpenChange: (open: boolean) => void
  rule: ModelHealthPolicyRule | null
  onSave: (rule: ModelHealthPolicyRule) => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [models, setModels] = useState('')
  const [regex, setRegex] = useState('')
  const [threshold, setThreshold] = useState(2)
  const [cooldown, setCooldown] = useState(300)
  const [statusCodes, setStatusCodes] = useState('404,429,500,502,503,504')
  const [groups, setGroups] = useState('')
  const [enabled, setEnabled] = useState(true)

  useEffect(() => {
    if (!props.open) return
    setName(props.rule?.name || '')
    setModels((props.rule?.models || []).join(', '))
    setRegex((props.rule?.model_regex || []).join(', '))
    setThreshold(props.rule?.failure_threshold || 2)
    setCooldown(props.rule?.cooldown_seconds || 300)
    setStatusCodes(
      (props.rule?.status_codes || [404, 429, 500, 502, 503, 504]).join(',')
    )
    setGroups((props.rule?.groups || []).join(', '))
    setEnabled(props.rule?.enabled !== false)
  }, [props.open, props.rule])

  const submit = () => {
    const modelList = parseList(models)
    const regexList = parseList(regex)
    if (!name.trim()) {
      toast.error(t('Name is required'))
      return
    }
    if (modelList.length === 0 && regexList.length === 0) {
      toast.error(t('Enter at least one model or model regex'))
      return
    }
    const codes = parseList(statusCodes).map(Number)
    if (codes.some((code) => !Number.isInteger(code) || code < 100 || code > 599)) {
      toast.error(t('Status codes must be numbers between 100 and 599'))
      return
    }
    props.onSave({
      id: props.rule?.id,
      name: name.trim(),
      enabled,
      models: modelList,
      model_regex: regexList,
      failure_threshold: threshold,
      cooldown_seconds: cooldown,
      status_codes: codes,
      groups: parseList(groups),
    })
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.rule ? t('Edit Rule') : t('Add Rule')}
      footer={
        <>
          <Button variant='outline' onClick={() => props.onOpenChange(false)}>
            {t('Cancel')}
          </Button>
          <Button onClick={submit}>{t('Save')}</Button>
        </>
      }
    >
      <div className='grid gap-4'>
        <div className='grid gap-1.5'>
          <Label>{t('Name')}</Label>
          <Input value={name} onChange={(event) => setName(event.target.value)} />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Models')}</Label>
          <Input
            value={models}
            placeholder='deepseek-v4.1-flash, glm-5.3-flash'
            onChange={(event) => setModels(event.target.value)}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Model Regex')}</Label>
          <Input
            value={regex}
            placeholder='^(deepseek|glm)-'
            onChange={(event) => setRegex(event.target.value)}
          />
        </div>
        <div className='grid grid-cols-2 gap-3'>
          <div className='grid gap-1.5'>
            <Label>{t('Failure threshold')}</Label>
            <Input
              type='number'
              min={1}
              value={threshold}
              onChange={(event) => setThreshold(Number(event.target.value))}
            />
          </div>
          <div className='grid gap-1.5'>
            <Label>{t('Cooldown seconds')}</Label>
            <Input
              type='number'
              min={1}
              value={cooldown}
              onChange={(event) => setCooldown(Number(event.target.value))}
            />
          </div>
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Status codes')}</Label>
          <Input
            value={statusCodes}
            onChange={(event) => setStatusCodes(event.target.value)}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Groups')}</Label>
          <Input
            value={groups}
            placeholder={t('Leave empty to match every group')}
            onChange={(event) => setGroups(event.target.value)}
          />
        </div>
        <div className='flex items-center justify-between'>
          <Label>{t('Enabled')}</Label>
          <Switch checked={enabled} onCheckedChange={setEnabled} />
        </div>
      </div>
    </Dialog>
  )
}
