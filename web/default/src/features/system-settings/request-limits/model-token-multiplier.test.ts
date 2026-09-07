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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  modelTokenMultiplierRowsSchema,
  parseModelTokenMultipliers,
  serializeModelTokenMultipliers,
} from './model-token-multiplier.ts'

describe('model token multiplier settings', () => {
  test('parses and serializes multiple models deterministically', () => {
    const rows = parseModelTokenMultipliers('{"model-b":2,"model-a":1.5}')

    assert.deepEqual(rows, [
      { model_id: 'model-a', multiplier: 1.5 },
      { model_id: 'model-b', multiplier: 2 },
    ])
    assert.equal(
      serializeModelTokenMultipliers(rows),
      '{"model-a":1.5,"model-b":2}'
    )
    assert.equal(serializeModelTokenMultipliers([]), '{}')
  })

  test('rejects duplicate model IDs after trimming', () => {
    const result = modelTokenMultiplierRowsSchema.safeParse([
      { model_id: 'model-a', multiplier: 2 },
      { model_id: ' model-a ', multiplier: 3 },
    ])

    assert.equal(result.success, false)
  })

  test('rejects non-positive multipliers', () => {
    const result = modelTokenMultiplierRowsSchema.safeParse([
      { model_id: 'model-a', multiplier: 0 },
    ])

    assert.equal(result.success, false)
  })
})
