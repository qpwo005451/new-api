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

interface MobileEmptyProps {
  title?: string
  description?: string
  className?: string
}

/**
 * Icon-free empty state for the mobile console. The desktop `EmptyState`
 * renders a `lucide-react` icon and an animated wrapper, both of which are on
 * the mobile bundle's forbidden list.
 */
export function MobileEmpty(props: MobileEmptyProps) {
  const { t } = useTranslation()

  return (
    <div
      className={cn(
        'flex min-h-[200px] flex-col items-center justify-center gap-1 px-4 text-center',
        props.className
      )}
    >
      <p className='text-sm font-medium'>{t(props.title ?? 'No Data')}</p>
      {props.description != null && (
        <p className='text-muted-foreground text-xs'>{props.description}</p>
      )}
    </div>
  )
}
