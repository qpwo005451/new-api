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

import { cn } from '@/lib/utils'
import { MOBILE_TABS } from '@/mobile/lib/router'
import type { MobileTab } from '@/mobile/types'

const TAB_LABEL_KEY: Record<MobileTab, string> = {
  usage: 'Usage',
  models: 'Model availability',
  routing: 'Routing statistics',
  channels: 'Channels',
  'routing-weights': 'Model routing',
}

interface MobileTabBarProps {
  active: MobileTab
  onChange: (tab: MobileTab) => void
}

export function MobileTabBar(props: MobileTabBarProps) {
  const { t } = useTranslation()

  return (
    <nav
      role='tablist'
      aria-label={t('Mobile console')}
      className='bg-background/95 fixed inset-x-0 bottom-0 flex border-t pb-[env(safe-area-inset-bottom)] backdrop-blur'
    >
      {MOBILE_TABS.map((tab) => {
        const selected = tab === props.active
        return (
          <button
            key={tab}
            type='button'
            role='tab'
            aria-selected={selected}
            className={cn(
              'flex min-h-12 flex-1 flex-col items-center justify-center gap-0.5 text-[11px]',
              selected ? 'text-foreground font-medium' : 'text-muted-foreground'
            )}
            onClick={() => props.onChange(tab)}
          >
            {t(TAB_LABEL_KEY[tab])}
          </button>
        )
      })}
    </nav>
  )
}
