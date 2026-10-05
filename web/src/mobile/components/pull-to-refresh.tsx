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
import { type ReactNode, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

// Long enough to be deliberate, short enough to reach with one thumb swipe.
const TRIGGER_DISTANCE = 64

interface PullToRefreshProps {
  onRefresh: () => Promise<void> | void
  children: ReactNode
}

// The wrapper is the scroll container (h-dvh), not the document, so the
// "is the list still at the top?" check reflects the element the finger is
// actually scrolling.
export function PullToRefresh(props: PullToRefreshProps) {
  const { t } = useTranslation()
  const startY = useRef<number | null>(null)
  const pulledFarEnough = useRef(false)
  const [refreshing, setRefreshing] = useState(false)

  const resetGesture = () => {
    startY.current = null
    pulledFarEnough.current = false
  }

  const handleTouchStart = (event: React.TouchEvent<HTMLDivElement>) => {
    // A gesture that starts away from the top is scrolling the list, not
    // pulling it down, so it must not arm a refresh.
    const atTop = event.currentTarget.scrollTop <= 0
    startY.current = atTop ? (event.touches[0]?.clientY ?? null) : null
    pulledFarEnough.current = false
  }

  const handleTouchMove = (event: React.TouchEvent<HTMLDivElement>) => {
    const start = startY.current
    const currentY = event.touches[0]?.clientY
    if (start === null || currentY === undefined || refreshing) {
      return
    }
    if (currentY - start > TRIGGER_DISTANCE) {
      pulledFarEnough.current = true
    }
  }

  const handleTouchCancel = () => {
    // The browser cancels the gesture when it takes the touch over (scroll
    // chaining, a system interruption). A touchEnd may still follow, so the
    // armed threshold has to be cleared here or that end would fire a refresh.
    resetGesture()
  }

  const handleTouchEnd = async () => {
    // The gesture is judged on release: starting the request while the finger
    // is still down would refetch on a pull the user then drags back.
    const shouldRefresh =
      startY.current !== null && pulledFarEnough.current && !refreshing
    resetGesture()
    if (!shouldRefresh) {
      return
    }
    setRefreshing(true)
    try {
      await props.onRefresh()
    } catch {
      // invalidateQueries resolves even when a refetch fails, so a failed pull
      // has no toast of its own: each page renders that query's own error
      // state. This catch only keeps an unexpected rejection from leaving the
      // indicator stuck and from surfacing as an unhandled rejection.
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <div
      className='h-dvh overflow-y-auto overscroll-contain'
      data-refreshing={refreshing}
      onTouchStart={handleTouchStart}
      onTouchMove={handleTouchMove}
      onTouchEnd={handleTouchEnd}
      onTouchCancel={handleTouchCancel}
    >
      {/* Always mounted so assistive technology announces the change; the
          height animates the visible indicator in and out. */}
      <div
        role='status'
        aria-live='polite'
        className='bg-background text-muted-foreground flex items-center justify-center overflow-hidden text-xs transition-[height]'
        style={{ height: refreshing ? '2rem' : 0 }}
      >
        {refreshing ? t('Refreshing...') : null}
      </div>
      {props.children}
    </div>
  )
}
