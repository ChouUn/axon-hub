import { Fragment, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Award, ChevronDown, ChevronRight, Medal, Trophy } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { formatDuration } from '@/utils/format-duration';
import { formatNumber } from '@/utils/format-number';
import type {
  AnalyticsModelChannelStat,
  AnalyticsModelStat,
} from '../data/analytics';

interface ModelAnalyticsTableProps {
  data: AnalyticsModelStat[];
  isLoading: boolean;
  currencyCode: string;
}

function RankIcon({ rank }: { rank: number }) {
  if (rank === 1) return <Trophy className='h-4 w-4 text-amber-500' />;
  if (rank === 2) return <Medal className='h-4 w-4 text-slate-400' />;
  if (rank === 3) return <Award className='h-4 w-4 text-orange-500' />;
  return <span className='text-xs text-muted-foreground'>#{rank}</span>;
}

export function ModelAnalyticsTable({
  data,
  isLoading,
  currencyCode,
}: ModelAnalyticsTableProps) {
  const { t, i18n } = useTranslation();
  const [expandedModels, setExpandedModels] = useState<Set<string>>(new Set());
  const locale = i18n.language.startsWith('zh') ? 'zh-CN' : 'en-US';

  useEffect(() => {
    setExpandedModels((current) => {
      const available = new Set(data.map((item) => item.id));
      return new Set(Array.from(current).filter((id) => available.has(id)));
    });
  }, [data]);

  const formatCurrency = (value: number) =>
    t('currencies.format', {
      val: value,
      currency: currencyCode,
      locale,
      minimumFractionDigits: 2,
      maximumFractionDigits: 4,
    });

  const formatLatency = (value: number | null) =>
    value == null ? '-' : formatDuration(value);

  const formatOutputSpeed = (value: number | null) =>
    value == null
      ? '-'
      : `${formatNumber(value, { digits: 2 })} tok/s`;

  const toggleExpanded = (id: string) => {
    setExpandedModels((current) => {
      const next = new Set(current);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  };

  const renderMetrics = (item: AnalyticsModelStat | AnalyticsModelChannelStat) => (
    <>
      <TableCell className='text-right'>
        {formatNumber(item.requestCount, { digits: 2 })}
      </TableCell>
      <TableCell className='text-right'>{formatCurrency(item.cost)}</TableCell>
      <TableCell className='text-right'>
        {formatNumber(item.totalTokens, { digits: 2 })}
      </TableCell>
      <TableCell className='text-right'>
        {formatCurrency(item.costPerMillion)}
      </TableCell>
      <TableCell className='text-right'>
        {formatNumber(item.successRate, { digits: 2 })}%
      </TableCell>
      <TableCell className='text-right'>
        {formatLatency(item.avgFirstTokenLatencyMs)}
      </TableCell>
      <TableCell className='text-right'>
        {formatOutputSpeed(item.avgOutputTokensPerSecond)}
      </TableCell>
    </>
  );

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>{t('analytics.modelAnalytics.table.title')}</CardTitle>
        </CardHeader>
        <CardContent>
          <Skeleton className='h-[420px] w-full' />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('analytics.modelAnalytics.table.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        {data.length === 0 ? (
          <div
            className={cn(
              'flex h-[240px] items-center justify-center',
              'text-sm text-muted-foreground'
            )}
          >
            {t('analytics.modelAnalytics.table.noData')}
          </div>
        ) : (
          <div className='overflow-x-auto'>
            <Table className='min-w-[1380px]'>
              <TableHeader>
                <TableRow>
                  <TableHead className='w-20 text-center'>
                    {t('analytics.modelAnalytics.table.rank')}
                  </TableHead>
                  <TableHead className='min-w-64'>
                    {t('analytics.modelAnalytics.table.name')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('analytics.modelAnalytics.table.requests')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('analytics.modelAnalytics.table.cost')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('analytics.modelAnalytics.table.tokens')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('analytics.modelAnalytics.table.costPerMillion')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('analytics.modelAnalytics.table.successRate')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('analytics.modelAnalytics.table.avgTTFB')}
                  </TableHead>
                  <TableHead className='text-right'>
                    {t('analytics.modelAnalytics.table.avgOutputSpeed')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((model, index) => {
                  const rank = index + 1;
                  const isExpanded = expandedModels.has(model.id);
                  const label = isExpanded
                    ? t('analytics.modelAnalytics.table.collapse', {
                        name: model.name,
                      })
                    : t('analytics.modelAnalytics.table.expand', {
                        name: model.name,
                      });

                  return (
                    <Fragment key={model.id}>
                      <TableRow className='bg-muted/20 font-medium'>
                        <TableCell className='text-center'>
                          <div className='flex items-center justify-center gap-1.5'>
                            <RankIcon rank={rank} />
                          </div>
                        </TableCell>
                        <TableCell className='max-w-96'>
                          <button
                            type='button'
                            className={cn(
                              'flex max-w-full items-center gap-2 text-left',
                              'hover:text-primary'
                            )}
                            aria-expanded={isExpanded}
                            aria-label={label}
                            title={label}
                            onClick={() => toggleExpanded(model.id)}
                          >
                            {isExpanded ? (
                              <ChevronDown className='h-4 w-4 shrink-0' />
                            ) : (
                              <ChevronRight className='h-4 w-4 shrink-0' />
                            )}
                            <span className='truncate'>{model.name}</span>
                          </button>
                        </TableCell>
                        {renderMetrics(model)}
                      </TableRow>
                      {isExpanded &&
                        model.channels.map((channel) => (
                          <TableRow
                            key={`${model.id}-${channel.id}`}
                            className='bg-background'
                          >
                            <TableCell />
                            <TableCell className='max-w-96 pl-12'>
                              <span className='block truncate'>{channel.name}</span>
                            </TableCell>
                            {renderMetrics(channel)}
                          </TableRow>
                        ))}
                    </Fragment>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
