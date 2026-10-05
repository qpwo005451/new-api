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
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { Toaster } from 'sonner'

import { ThemeProvider, useTheme } from '@/context/theme-provider'
import { mobileQueryClient } from '@/mobile/lib/query-client'

interface MobileProvidersProps {
  children: ReactNode
}

// Must render inside ThemeProvider: the mobile console follows the resolved
// system/desktop preference, so a dark system gets dark toasts.
function ThemedToaster() {
  const { resolvedTheme } = useTheme()

  return <Toaster theme={resolvedTheme} />
}

export function MobileProviders(props: MobileProvidersProps) {
  return (
    <QueryClientProvider client={mobileQueryClient}>
      <ThemeProvider>
        {props.children}
        {/* Plain sonner, not @/components/ui/sonner: that wrapper pulls
            @hugeicons/* and the desktop theme provider into the mobile bundle. */}
        <ThemedToaster />
      </ThemeProvider>
    </QueryClientProvider>
  )
}
