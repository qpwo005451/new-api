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
import { Activity, Play, Plus, RefreshCw, Save } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Button } from '@/components/design-system/button'
import { Input } from '@/components/design-system/input'
import { SectionPageLayout } from '@/components/layout'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'

import {
  enqueueModelMonitorRun,
  getModelMonitorConfig,
  getModelMonitorSites,
  updateModelMonitorConfig,
} from './api'
import { MonitorTasksPanel } from './components/monitor-tasks-panel'
import { RunMonitorDialog } from './components/run-monitor-dialog'
import { SiteDetailDialog } from './components/site-detail-dialog'
import { SiteEditor } from './components/site-editor'
import { SiteSummaryCard } from './components/site-summary-card'
import type { ModelMonitorConfig, ModelMonitorSiteConfig } from './types'

function createSite(): ModelMonitorSiteConfig {
  return {
    name: '',
    site_type: 'newapi',
    pricing_group: '',
    enabled: true,
    channel_ids: [],
    targets: [],
  }
}

export function ModelMonitor() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<ModelMonitorConfig | null>(null)
  const [showEditor, setShowEditor] = useState(false)
  const [siteId, setSiteId] = useState<number | null>(null)
  const [runDialogOpen, setRunDialogOpen] = useState(false)
  const configQuery = useQuery({
    queryKey: ['model-monitor', 'config'],
    queryFn: getModelMonitorConfig,
    retry: false,
  })
  const sitesQuery = useQuery({
    queryKey: ['model-monitor', 'sites'],
    queryFn: getModelMonitorSites,
    retry: false,
    refetchInterval: 30_000,
  })
  const config = draft ?? configQuery.data?.data
  const sites = sitesQuery.data?.data ?? []

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['model-monitor', 'config'] }),
      queryClient.invalidateQueries({ queryKey: ['model-monitor', 'sites'] }),
      queryClient.invalidateQueries({ queryKey: ['model-monitor', 'tasks'] }),
    ])
  }

  const saveMutation = useMutation({
    mutationFn: updateModelMonitorConfig,
    onSuccess: async (response) => {
      if (!response.success || !response.data) {
        toast.error(
          response.message || t('Failed to save model monitor settings')
        )
        return
      }
      setDraft(response.data)
      await invalidate()
      toast.success(t('Model monitor settings saved'))
    },
  })
  const runMutation = useMutation({
    mutationFn: enqueueModelMonitorRun,
    onSuccess: async (response) => {
      if (!response.success) {
        toast.error(response.message || t('Failed to start monitor run'))
        return
      }
      setRunDialogOpen(false)
      toast.success(
        response.data?.created
          ? t('Monitor run queued')
          : t('An existing monitor run is already active')
      )
      await invalidate()
    },
  })

  const replaceSite = (index: number, site: ModelMonitorSiteConfig) => {
    if (!config) return
    const nextSites = [...config.sites]
    nextSites[index] = site
    setDraft({ ...config, sites: nextSites })
  }

  if (configQuery.isLoading) {
    return (
      <div className='text-muted-foreground p-6 text-sm'>{t('Loading...')}</div>
    )
  }

  if (!config || !configQuery.data?.success) {
    return (
      <div className='text-destructive p-6 text-sm'>
        {configQuery.data?.message ||
          t('Failed to load model monitor settings')}
      </div>
    )
  }

  return (
    <>
      <SectionPageLayout fixedContent={false}>
        <SectionPageLayout.Title>
          <span className='flex items-center gap-2'>
            <Activity className='size-5' aria-hidden='true' />
            {t('Model Availability')}
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Actions>
          <Button
            type='button'
            variant='outline'
            onClick={() => void invalidate()}
            disabled={sitesQuery.isFetching}
          >
            <RefreshCw
              data-icon='inline-start'
              className={sitesQuery.isFetching ? 'animate-spin' : undefined}
              aria-hidden='true'
            />
            {t('Refresh')}
          </Button>
          <Button
            type='button'
            variant='outline'
            onClick={() => setRunDialogOpen(true)}
            disabled={!config.setting.enabled || runMutation.isPending}
          >
            <Play data-icon='inline-start' aria-hidden='true' />
            {t('Run monitor')}
          </Button>
          <Button
            type='button'
            onClick={() => saveMutation.mutate(config)}
            disabled={saveMutation.isPending}
          >
            <Save data-icon='inline-start' aria-hidden='true' />
            {saveMutation.isPending
              ? t('Saving...')
              : t('Save configuration')}
          </Button>
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            <Card>
              <CardHeader>
                <CardTitle>{t('Monitoring controls')}</CardTitle>
                <CardDescription>
                  {t(
                    'Passive observations never change channel routing. Active runs send controlled upstream checks.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='grid gap-4 md:grid-cols-2 xl:grid-cols-4'>
                <label className='flex items-center justify-between gap-3 rounded-lg border p-3'>
                  <span>
                    <span className='block font-medium'>
                      {t('Monitoring enabled')}
                    </span>
                    <span className='text-muted-foreground text-xs'>
                      {t('Collect passive observations')}
                    </span>
                  </span>
                  <Switch
                    checked={config.setting.enabled}
                    onCheckedChange={(enabled) =>
                      setDraft({
                        ...config,
                        setting: { ...config.setting, enabled },
                      })
                    }
                  />
                </label>
                <label className='flex items-center justify-between gap-3 rounded-lg border p-3'>
                  <span>
                    <span className='block font-medium'>
                      {t('Scheduled probes')}
                    </span>
                    <span className='text-muted-foreground text-xs'>
                      {t('Run controlled upstream checks')}
                    </span>
                  </span>
                  <Switch
                    checked={config.setting.auto_probe_enabled}
                    onCheckedChange={(auto_probe_enabled) =>
                      setDraft({
                        ...config,
                        setting: { ...config.setting, auto_probe_enabled },
                      })
                    }
                  />
                </label>
                <label className='grid gap-1.5 text-sm'>
                  <span className='font-medium'>
                    {t('Probe interval (minutes)')}
                  </span>
                  <Input
                    type='number'
                    min={1}
                    value={config.setting.auto_probe_interval_minutes}
                    onChange={(event) =>
                      setDraft({
                        ...config,
                        setting: {
                          ...config.setting,
                          auto_probe_interval_minutes: Number(
                            event.target.value
                          ),
                        },
                      })
                    }
                  />
                </label>
                <label className='grid gap-1.5 text-sm'>
                  <span className='font-medium'>
                    {t('Unknown grace (minutes)')}
                  </span>
                  <Input
                    type='number'
                    min={1}
                    value={config.setting.unknown_grace_minutes}
                    onChange={(event) =>
                      setDraft({
                        ...config,
                        setting: {
                          ...config.setting,
                          unknown_grace_minutes: Number(event.target.value),
                        },
                      })
                    }
                  />
                </label>
              </CardContent>
            </Card>

            <div className='grid gap-3 lg:grid-cols-2'>
              {sites.map((site) => (
                <SiteSummaryCard
                  key={site.site.id}
                  site={site}
                  onOpen={() => setSiteId(site.site.id)}
                />
              ))}
            </div>

            <MonitorTasksPanel />

            <Card>
              <CardHeader>
                <CardTitle>{t('Site configuration')}</CardTitle>
                <CardDescription>
                  {t(
                    'Changes keep existing monitor history. Removing a target disables it instead of deleting its observations.'
                  )}
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-3'>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    type='button'
                    variant='outline'
                    onClick={() => setShowEditor((value) => !value)}
                  >
                    {showEditor ? t('Hide editor') : t('Edit configuration')}
                  </Button>
                  {showEditor && (
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() =>
                        setDraft({
                          ...config,
                          sites: [...config.sites, createSite()],
                        })
                      }
                    >
                      <Plus data-icon='inline-start' aria-hidden='true' />
                      {t('Add site')}
                    </Button>
                  )}
                </div>
                {showEditor && (
                  <div className='space-y-3'>
                    {config.sites.map((site, index) => (
                      <SiteEditor
                        key={site.id ?? `new-${index}`}
                        site={site}
                        onChange={(next) => replaceSite(index, next)}
                        onRemove={() =>
                          setDraft({
                            ...config,
                            sites: config.sites.filter(
                              (_, itemIndex) => itemIndex !== index
                            ),
                          })
                        }
                      />
                    ))}
                  </div>
                )}
              </CardContent>
            </Card>
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>
      <RunMonitorDialog
        open={runDialogOpen}
        pending={runMutation.isPending}
        onOpenChange={setRunDialogOpen}
        onConfirm={() => runMutation.mutate()}
      />
      <SiteDetailDialog
        siteId={siteId}
        onOpenChange={(open) => !open && setSiteId(null)}
      />
    </>
  )
}
