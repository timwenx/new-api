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
import { AlertCircleIcon, GlobalIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useQuery } from '@tanstack/react-query'
import { type ReactNode, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'
import {
  sideDrawerContentClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Skeleton } from '@/components/ui/skeleton'
import { formatLogQuota, formatTimestamp } from '@/lib/format'

import { getUserIPUsage } from '../../api'
import type { UserIPUsage } from '../../types'

const PAGE_SIZE = 20
const SKELETON_ROWS = [0, 1, 2, 3] as const

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  user: { id: number; username?: string } | null
}

export function UserIPUsageDialog(props: Props) {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)

  const usageQuery = useQuery({
    queryKey: ['user-ip-usage', props.user?.id, page],
    queryFn: async () => {
      if (!props.user) {
        throw new Error('Missing user')
      }
      const result = await getUserIPUsage(props.user.id, {
        p: page,
        page_size: PAGE_SIZE,
      })
      if (!result.success || !result.data) {
        throw new Error(result.message || 'Failed to load IP usage')
      }
      return result.data
    },
    enabled: props.open && props.user !== null,
    staleTime: 30_000,
  })

  const items = usageQuery.data?.items ?? []
  const total = usageQuery.data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setPage(1)
    }
    props.onOpenChange(open)
  }

  let content: ReactNode
  if (usageQuery.isLoading) {
    content = (
      <div className='space-y-3' aria-label={t('Loading...')}>
        {SKELETON_ROWS.map((row) => (
          <Skeleton key={row} className='h-14 w-full' />
        ))}
      </div>
    )
  } else if (usageQuery.isError) {
    content = (
      <Empty className='min-h-52 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={AlertCircleIcon} strokeWidth={2} />
          </EmptyMedia>
          <EmptyTitle>{t('Failed to load IP usage')}</EmptyTitle>
        </EmptyHeader>
        <EmptyContent>
          <Button
            variant='outline'
            onClick={() => usageQuery.refetch()}
            disabled={usageQuery.isFetching}
          >
            {t('Retry')}
          </Button>
        </EmptyContent>
      </Empty>
    )
  } else if (items.length === 0) {
    content = (
      <Empty className='min-h-52 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon'>
            <HugeiconsIcon icon={GlobalIcon} strokeWidth={2} />
          </EmptyMedia>
          <EmptyTitle>{t('No IP usage records')}</EmptyTitle>
          <EmptyDescription>
            {t('No request IPs have been recorded for this user.')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  } else {
    content = (
      <>
        <StaticDataTable<UserIPUsage>
          data={items}
          getRowKey={(record) => record.ip}
          className='overflow-x-auto'
          tableClassName='min-w-[760px]'
          columns={[
            {
              id: 'ip',
              header: t('IP'),
              className: 'min-w-36',
              cell: (record) => (
                <StatusBadge label={record.ip} variant='orange' size='sm' />
              ),
            },
            {
              id: 'requests',
              header: t('Requests'),
              className: 'text-right',
              cellClassName: 'text-right font-mono tabular-nums',
              cell: (record) => record.request_count.toLocaleString(),
            },
            {
              id: 'tokens',
              header: t('Total Tokens'),
              className: 'min-w-64',
              cell: (record) => (
                <div className='space-y-1'>
                  <div className='font-medium tabular-nums'>
                    {(
                      record.prompt_tokens + record.completion_tokens
                    ).toLocaleString()}
                  </div>
                  <div className='text-muted-foreground text-xs whitespace-nowrap'>
                    {t('Input Tokens')}: {record.prompt_tokens.toLocaleString()}{' '}
                    · {t('Output Tokens')}:{' '}
                    {record.completion_tokens.toLocaleString()}
                  </div>
                </div>
              ),
            },
            {
              id: 'quota',
              header: t('Used Quota'),
              className: 'min-w-28 text-right',
              cellClassName: 'text-right font-medium tabular-nums',
              cell: (record) => formatLogQuota(record.quota),
            },
            {
              id: 'last_used_at',
              header: t('Last Used'),
              className: 'min-w-40',
              cellClassName: 'text-muted-foreground whitespace-nowrap',
              cell: (record) => formatTimestamp(record.last_used_at),
            },
          ]}
        />

        <div className='flex flex-wrap items-center justify-between gap-3'>
          <div className='text-muted-foreground text-sm'>
            {t('Total:')} {total.toLocaleString()}
          </div>
          <div className='flex items-center gap-2'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setPage((current) => current - 1)}
              disabled={page <= 1 || usageQuery.isFetching}
            >
              {t('Previous')}
            </Button>
            <span className='text-muted-foreground min-w-24 text-center text-sm tabular-nums'>
              {t('Page {{current}} of {{total}}', {
                current: page,
                total: totalPages,
              })}
            </span>
            <Button
              variant='outline'
              size='sm'
              onClick={() => setPage((current) => current + 1)}
              disabled={page >= totalPages || usageQuery.isFetching}
            >
              {t('Next')}
            </Button>
          </div>
        </div>
      </>
    )
  }

  return (
    <Sheet open={props.open} onOpenChange={handleOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-4xl')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('IP Usage Details')}</SheetTitle>
          <SheetDescription>
            {props.user?.username || '-'} (ID: {props.user?.id || '-'})
          </SheetDescription>
        </SheetHeader>
        <div className={sideDrawerFormClassName('gap-4')}>{content}</div>
      </SheetContent>
    </Sheet>
  )
}
