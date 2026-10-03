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
import { useCallback, useEffect, useState } from 'react'

import { MOBILE_TABS, type MobileTab } from '@/mobile/types'

// Re-exported so the tab bar can keep importing the navigation order from the
// router; the list itself stays defined next to MobileTab in types.ts.
export { MOBILE_TABS }

export function parseTab(hash: string): MobileTab {
  const normalized = hash.replace(/^#\/?/, '').replace(/\/+$/, '')
  const candidate = MOBILE_TABS.find((tab) => tab === normalized)
  return candidate ?? 'usage'
}

export function tabHash(tab: MobileTab): string {
  return `#/${tab}`
}

export function useActiveTab(): [MobileTab, (tab: MobileTab) => void] {
  const [active, setActive] = useState<MobileTab>(() =>
    parseTab(window.location.hash)
  )

  useEffect(() => {
    const onHashChange = () => setActive(parseTab(window.location.hash))
    window.addEventListener('hashchange', onHashChange)
    return () => window.removeEventListener('hashchange', onHashChange)
  }, [])

  const select = useCallback((tab: MobileTab) => {
    window.location.hash = tabHash(tab)
  }, [])

  return [active, select]
}
