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
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import { MobileError } from '@/mobile/components/mobile-error'
import { MobileLoading } from '@/mobile/components/mobile-loading'
import { MobileTabBar } from '@/mobile/components/mobile-tab-bar'
import { PatGate } from '@/mobile/components/pat-gate'
import { PullToRefresh } from '@/mobile/components/pull-to-refresh'
import { ChannelsPage } from '@/mobile/features/channels/components/channels-page'
import { ModelsPage } from '@/mobile/features/models/components/models-page'
import { RoutingWeightsPage } from '@/mobile/features/routing-weights/components/routing-weights-page'
import { RoutingPage } from '@/mobile/features/routing/components/routing-page'
import { UsagePage } from '@/mobile/features/usage/components/usage-page'
import { mobileErrorCopy } from '@/mobile/lib/error-copy'
import { mobileQueryClient } from '@/mobile/lib/query-client'
import { useActiveTab } from '@/mobile/lib/router'
import { mobileStatusQueryOptions } from '@/mobile/lib/status'

export function MobileApp() {
  const { t } = useTranslation()
  const [activeTab, selectTab] = useActiveTab()
  // formatQuotaWithCurrency and the other money helpers read the currency
  // settings from the system-config store, which /api/status hydrates. Without
  // this query every amount would be rendered with the USD defaults.
  const status = useQuery(mobileStatusQueryOptions)

  if (status.isPending) {
    return <MobileLoading />
  }

  // `/api/status` is unauthenticated and cheap, so when it fails the console
  // almost certainly cannot reach the gateway at all. Say that here instead of
  // rendering the shell and letting every tab fail with a generic load error.
  if (status.isError) {
    const errorCopy = mobileErrorCopy(status.error)
    return (
      <div className='mx-auto flex min-h-dvh max-w-md flex-col justify-center px-5'>
        <MobileError
          title={t(errorCopy.titleKey)}
          description={t(errorCopy.descriptionKey)}
          onRetry={() => void status.refetch()}
        />
      </div>
    )
  }

  // Every mobile feature keys its queries under ['mobile', ...], so one prefix
  // invalidates the four tabs without racing their in-flight requests.
  const refreshAll = () =>
    mobileQueryClient.invalidateQueries({ queryKey: ['mobile'] })

  const openDesktopConsole = () => {
    // The backend reads this cookie on the next load and serves the desktop
    // shell instead of redirecting back to /m (see router/web-router.go).
    document.cookie =
      'newapi_prefer_desktop=1; path=/; max-age=31536000; samesite=lax'
    window.location.href = '/'
  }

  return (
    <PatGate onReady={() => {}}>
      <PullToRefresh onRefresh={refreshAll}>
        <main className='bg-background text-foreground min-h-dvh pb-[calc(4rem+env(safe-area-inset-bottom))]'>
          <header className='px-4 pt-[max(1rem,env(safe-area-inset-top))] pb-2'>
            <h1 className='text-lg font-semibold'>{t('Mobile console')}</h1>
          </header>
          {activeTab === 'usage' ? <UsagePage /> : null}
          {activeTab === 'models' ? <ModelsPage /> : null}
          {activeTab === 'routing' ? <RoutingPage /> : null}
          {activeTab === 'channels' ? <ChannelsPage /> : null}
          {activeTab === 'routing-weights' ? <RoutingWeightsPage /> : null}
          <div className='px-3 pb-2'>
            <button
              type='button'
              className='text-muted-foreground text-xs underline'
              onClick={openDesktopConsole}
            >
              {t('Desktop version')}
            </button>
          </div>
        </main>
      </PullToRefresh>
      <MobileTabBar active={activeTab} onChange={selectTab} />
    </PatGate>
  )
}
