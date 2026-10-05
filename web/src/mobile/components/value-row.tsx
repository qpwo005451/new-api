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
interface ValueRowProps {
  label: string
  value: string
  secondary?: string
}

export function ValueRow(props: ValueRowProps) {
  return (
    <div className='flex min-h-11 items-center justify-between gap-3 px-3 py-2'>
      <div className='min-w-0'>
        <p className='truncate text-sm'>{props.label}</p>
        {props.secondary ? (
          <p className='text-muted-foreground truncate text-xs'>
            {props.secondary}
          </p>
        ) : null}
      </div>
      <p className='shrink-0 text-sm font-medium tabular-nums'>{props.value}</p>
    </div>
  )
}
