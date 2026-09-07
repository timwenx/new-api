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
import * as z from 'zod'

export const MAX_MODEL_TOKEN_MULTIPLIER = 1_000_000

export const modelTokenMultiplierRowsSchema = z
  .array(
    z.object({
      model_id: z
        .string()
        .trim()
        .min(1, 'Model ID is required')
        .max(191, 'Model ID must not exceed 191 characters'),
      multiplier: z
        .number()
        .positive('Multiplier must be greater than 0')
        .max(MAX_MODEL_TOKEN_MULTIPLIER, 'Multiplier cannot exceed 1000000'),
    })
  )
  .superRefine((rows, context) => {
    const seen = new Set<string>()
    rows.forEach((row, index) => {
      const modelID = row.model_id.trim()
      if (seen.has(modelID)) {
        context.addIssue({
          code: 'custom',
          message: 'Duplicate model ID',
          path: [index, 'model_id'],
        })
      }
      seen.add(modelID)
    })
  })

export type ModelTokenMultiplierRow = z.output<
  typeof modelTokenMultiplierRowsSchema
>[number]

export function parseModelTokenMultipliers(
  rawValue: string
): ModelTokenMultiplierRow[] {
  try {
    const parsed = JSON.parse(rawValue) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return []
    }
    return Object.entries(parsed)
      .filter(
        (entry): entry is [string, number] =>
          typeof entry[1] === 'number' && Number.isFinite(entry[1])
      )
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([modelID, multiplier]) => ({
        model_id: modelID,
        multiplier,
      }))
  } catch {
    return []
  }
}

export function serializeModelTokenMultipliers(
  rows: ModelTokenMultiplierRow[]
): string {
  const entries = rows
    .map((row) => [row.model_id.trim(), row.multiplier] as const)
    .sort(([left], [right]) => left.localeCompare(right))
  return JSON.stringify(Object.fromEntries(entries))
}
