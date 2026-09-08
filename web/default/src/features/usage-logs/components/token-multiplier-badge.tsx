/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'

function formatMultiplier(multiplier: number): string {
  return String(Number(multiplier.toPrecision(12)))
}

export function TokenMultiplierBadge(props: {
  multiplier?: number
  title?: string
}) {
  const { t } = useTranslation()
  const multiplier = props.multiplier ?? 1
  if (!Number.isFinite(multiplier) || multiplier <= 0 || multiplier === 1) {
    return null
  }

  const label = `× ${formatMultiplier(multiplier)}`
  return (
    <Badge
      variant='secondary'
      title={`${props.title ?? t('Token multiplier')}: ${label}`}
    >
      {label}
    </Badge>
  )
}
