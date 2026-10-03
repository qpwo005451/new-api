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
import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { isPlausiblePat, readPat, writePat } from '@/mobile/lib/pat-store'

interface PatGateProps {
  onReady: (pat: string) => void
  children: ReactNode
}

export function PatGate(props: PatGateProps) {
  const { t } = useTranslation()
  const [storedPat, setStoredPat] = useState(() => readPat())
  const [draft, setDraft] = useState('')
  const [errorKey, setErrorKey] = useState<string | null>(null)

  if (storedPat !== '') {
    return props.children
  }

  const submit = () => {
    if (!isPlausiblePat(draft)) {
      setErrorKey('The token looks too short. Check that you copied all of it.')
      return
    }
    const token = draft.trim()
    writePat(token)
    setErrorKey(null)
    setStoredPat(token)
    props.onReady(token)
  }

  return (
    <main className='mx-auto flex min-h-dvh max-w-md flex-col justify-center gap-4 px-5 pb-[max(1rem,env(safe-area-inset-bottom))]'>
      <div className='space-y-1'>
        <h1 className='text-lg font-semibold'>{t('Access token required')}</h1>
        <p className='text-muted-foreground text-sm'>
          {t('Paste the access token you generated on the desktop console.')}
        </p>
      </div>
      <div className='space-y-2'>
        <Label htmlFor='mobile-pat'>{t('Access token')}</Label>
        <Input
          id='mobile-pat'
          autoComplete='off'
          inputMode='text'
          spellCheck={false}
          type='password'
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
          aria-invalid={errorKey !== null}
          aria-describedby={errorKey ? 'mobile-pat-error' : undefined}
        />
        {errorKey ? (
          <p
            id='mobile-pat-error'
            role='alert'
            className='text-destructive text-xs'
          >
            {t(errorKey)}
          </p>
        ) : null}
      </div>
      <Button className='h-11' onClick={submit}>
        {t('Verify and continue')}
      </Button>
    </main>
  )
}
