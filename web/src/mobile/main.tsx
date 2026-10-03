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
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { Toaster } from '@/components/ui/sonner'
import { ThemeProvider } from '@/context/theme-provider'
import '@/mobile/styles/mobile.css'

import { MobileApp } from '@/mobile/app'
import { initializeMobileI18n } from '@/mobile/lib/i18n'

const container = document.getElementById('root')
if (container) {
  // Not awaited: the shell renders with the English source keys first and
  // re-renders once the active locale chunk arrives.
  void initializeMobileI18n()

  createRoot(container).render(
    <StrictMode>
      <ThemeProvider>
        <MobileApp />
        <Toaster />
      </ThemeProvider>
    </StrictMode>
  )
}
