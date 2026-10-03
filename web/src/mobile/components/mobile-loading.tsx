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

interface MobileLoadingProps {
  className?: string
}

/**
 * Icon-free loading indicator for the mobile console. The desktop
 * `LoadingState` pulls in `lucide-react`, which the mobile bundle forbids.
 */
export function MobileLoading(props: MobileLoadingProps) {
  const { t } = useTranslation()

  return (
    <div
      role='status'
      aria-live='polite'
      className={cn(
        'flex min-h-[200px] items-center justify-center',
        props.className
      )}
    >
      <span
        aria-hidden='true'
        className='border-muted-foreground/30 border-t-foreground size-5 animate-spin rounded-full border-2'
      />
      <span className='sr-only'>{t('Loading...')}</span>
    </div>
  )
}
