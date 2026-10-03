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

import { MobileLoading } from '@/mobile/components/mobile-loading'
import { PatGate } from '@/mobile/components/pat-gate'
import { mobileStatusQueryOptions } from '@/mobile/lib/status'

export function MobileApp() {
  const { t } = useTranslation()
  // formatQuotaWithCurrency and the other money helpers read the currency
  // settings from the system-config store, which /api/status hydrates. Without
  // this query every amount would be rendered with the USD defaults.
  const status = useQuery(mobileStatusQueryOptions)

  if (status.isPending) {
    return <MobileLoading />
  }

  return (
    <PatGate onReady={() => {}}>
      <main className='bg-background text-foreground min-h-dvh'>
        <header className='px-4 pt-[max(1rem,env(safe-area-inset-top))] pb-2'>
          <h1 className='text-lg font-semibold'>{t('Mobile console')}</h1>
        </header>
      </main>
    </PatGate>
  )
}
