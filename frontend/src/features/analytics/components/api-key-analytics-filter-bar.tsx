import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { IconFilter, IconX } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { AnalyticsFacetedFilter } from './analytics-faceted-filter';
import { DateRangePicker, formatDate } from './analytics-filter-bar';
import type { AnalyticsAPIKeyTemplate } from '../data/analytics';

interface APIKeyAnalyticsFilterBarProps {
  earliestDate?: string | null;
  startTime: string | null;
  endTime: string | null;
  templateIDs: string[];
  templates: AnalyticsAPIKeyTemplate[];
  isLoadingTemplates: boolean;
  onStartTimeChange: (time: string | null) => void;
  onEndTimeChange: (time: string | null) => void;
  onTemplateIDsChange: (ids: string[]) => void;
  onReset: () => void;
}

export function APIKeyAnalyticsFilterBar({
  earliestDate,
  startTime,
  endTime,
  templateIDs,
  templates,
  isLoadingTemplates,
  onStartTimeChange,
  onEndTimeChange,
  onTemplateIDsChange,
  onReset,
}: APIKeyAnalyticsFilterBarProps) {
  const { t } = useTranslation();

  const setQuickDay = useCallback(
    (daysAgo: number) => {
      const date = new Date();
      date.setDate(date.getDate() - daysAgo);
      date.setHours(0, 0, 0, 0);
      const formattedDate = formatDate(date);
      onStartTimeChange(formattedDate);
      onEndTimeChange(formattedDate);
    },
    [onEndTimeChange, onStartTimeChange]
  );

  const setQuickWeek = useCallback(() => {
    const now = new Date();
    const start = new Date(now);
    const daysSinceMonday = (start.getDay() + 6) % 7;
    start.setDate(start.getDate() - daysSinceMonday);
    start.setHours(0, 0, 0, 0);
    onStartTimeChange(formatDate(start));
    onEndTimeChange(formatDate(now));
  }, [onEndTimeChange, onStartTimeChange]);

  const setQuickMonth = useCallback(() => {
    const now = new Date();
    onStartTimeChange(formatDate(new Date(now.getFullYear(), now.getMonth(), 1)));
    onEndTimeChange(formatDate(now));
  }, [onEndTimeChange, onStartTimeChange]);

  const setAllTime = useCallback(() => {
    onStartTimeChange(earliestDate ?? null);
    onEndTimeChange(earliestDate ? formatDate(new Date()) : null);
  }, [earliestDate, onEndTimeChange, onStartTimeChange]);

  const hasFilters = Boolean(startTime || endTime || templateIDs.length > 0);

  return (
    <div className='space-y-3 rounded-lg border bg-card p-4'>
      <div className='flex flex-wrap items-center gap-2'>
        <div className='flex items-center gap-1.5 text-sm font-medium'>
          <IconFilter className='h-4 w-4 text-muted-foreground' />
          {t('analytics.filter.dateRange')}
        </div>
        <DateRangePicker
          startDate={startTime}
          endDate={endTime}
          onStartChange={(date) => onStartTimeChange(date ? formatDate(date) : null)}
          onEndChange={(date) => onEndTimeChange(date ? formatDate(date) : null)}
        />
        <div className='flex flex-wrap items-center gap-1'>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={() => setQuickDay(0)}
          >
            {t('analytics.filter.today')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={() => setQuickDay(1)}
          >
            {t('analytics.apiKeyAnalytics.filter.yesterday')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={setQuickWeek}
          >
            {t('analytics.apiKeyAnalytics.filter.thisWeek')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={setQuickMonth}
          >
            {t('analytics.filter.thisMonth')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={setAllTime}
          >
            {t('analytics.apiKeyAnalytics.filter.all')}
          </Button>
        </div>
      </div>

      <div className='flex flex-wrap items-center gap-2'>
        <AnalyticsFacetedFilter
          title={t('analytics.apiKeyAnalytics.filter.template')}
          options={templates.map((template) => ({
            label: template.name,
            value: template.id,
          }))}
          selectedValues={templateIDs}
          onSelectedValuesChange={onTemplateIDsChange}
          isLoading={isLoadingTemplates}
        />
        {hasFilters && (
          <Button
            variant='ghost'
            size='sm'
            className='h-8 text-xs text-muted-foreground'
            onClick={onReset}
          >
            <IconX className='mr-1 h-3 w-3' />
            {t('analytics.filter.reset')}
          </Button>
        )}
      </div>
    </div>
  );
}
