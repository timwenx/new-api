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
import { useFieldArray, useFormContext } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'

import { SettingsControlGroup } from '../components/settings-form-layout'
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  MAX_MODEL_TOKEN_MULTIPLIER,
  type ModelTokenMultiplierRow,
} from './model-token-multiplier'

type ModelTokenMultiplierFormValues = {
  token_setting: {
    model_token_multipliers: ModelTokenMultiplierRow[]
  }
}

export function ModelTokenMultiplierFields() {
  const { t } = useTranslation()
  const form = useFormContext<ModelTokenMultiplierFormValues>()
  const rows = useFieldArray({
    control: form.control,
    name: 'token_setting.model_token_multipliers',
  })

  return (
    <SettingsControlGroup>
      <div className='flex flex-wrap items-start justify-between gap-3'>
        <div className='flex min-w-0 flex-col gap-1'>
          <p className='text-sm font-medium'>{t('Model token multipliers')}</p>
          <p className='text-muted-foreground text-sm'>
            {t(
              'Configure exact model IDs and their token counting multipliers. This affects daily and weekly token limits and usage dashboards, but not billing prices.'
            )}
          </p>
        </div>
        <Button
          type='button'
          variant='outline'
          size='sm'
          onClick={() => rows.append({ model_id: '', multiplier: 1 })}
        >
          {t('Add model multiplier')}
        </Button>
      </div>

      {rows.fields.length === 0 ? (
        <p className='text-muted-foreground text-sm'>
          {t('No model multipliers configured.')}
        </p>
      ) : (
        <div className='flex flex-col gap-3'>
          {rows.fields.map((row, index) => (
            <div
              key={row.id}
              className='grid items-start gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(8rem,12rem)_auto]'
            >
              <FormField
                control={form.control}
                name={`token_setting.model_token_multipliers.${index}.model_id`}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Model ID')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder='gpt-...' />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name={`token_setting.model_token_multipliers.${index}.multiplier`}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Token multiplier')}</FormLabel>
                    <FormControl>
                      <Input
                        type='number'
                        min={0}
                        max={MAX_MODEL_TOKEN_MULTIPLIER}
                        step='any'
                        {...safeNumberFieldProps(field)}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='sm:mt-7'
                onClick={() => rows.remove(index)}
                aria-label={t('Delete')}
              >
                {t('Delete')}
              </Button>
            </div>
          ))}
        </div>
      )}
    </SettingsControlGroup>
  )
}
