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

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface MobileErrorProps {
  title?: string
  description?: string
  onRetry?: () => void
  className?: string
}

/**
 * Icon-free error state for the mobile console. The desktop `ErrorState`
 * renders a `lucide-react` warning icon and an animated wrapper, both of which
 * are on the mobile bundle's forbidden list.
 */
export function MobileError(props: MobileErrorProps) {
  const { t } = useTranslation()

  return (
    <div
      role='alert'
      className={cn(
        'flex min-h-[200px] flex-col items-center justify-center gap-1 px-4 text-center',
        props.className
      )}
    >
      <p className='text-sm font-medium'>
        {t(props.title ?? 'Oops! Something went wrong')}
      </p>
      {props.description != null && (
        <p className='text-muted-foreground text-xs'>{props.description}</p>
      )}
      {props.onRetry != null && (
        <Button className='mt-3 h-10' onClick={props.onRetry}>
          {t('Retry')}
        </Button>
      )}
    </div>
  )
}
