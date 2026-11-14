import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCalendar, IconFilter, IconX } from '@tabler/icons-react';
import { Button } from '@/components/ui/button';
import { AnalyticsFacetedFilter } from './analytics-faceted-filter';
import { toast } from 'sonner';
import { cn } from '@/lib/utils';
import { Calendar } from '@/components/ui/calendar';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import {
  getAnalyticsQuickDateRange,
  type AnalyticsQuickRange,
} from '../utils/date-range';
import type { AnalyticsAPIKeyTemplate } from '../data/analytics';

// Calendar Date → 'YYYY-MM-DD' 字符串（直接取本地年月日，不做时区转换）
function formatDate(date: Date): string {
  const y = date.getFullYear();
  const m = String(date.getMonth() + 1).padStart(2, '0');
  const d = String(date.getDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

// 'YYYY-MM-DD' → Date（用于 Calendar selected）
function parseDate(dateStr: string): Date {
  const [y, m, d] = dateStr.split('-').map(Number);
  return new Date(y, m - 1, d);
}

interface DateRangePickerProps {
  startDate: string | null;
  endDate: string | null;
  onStartChange: (date: Date | null) => void;
  onEndChange: (date: Date | null) => void;
}

function DateRangePicker({
  startDate,
  endDate,
  onStartChange,
  onEndChange,
}: DateRangePickerProps) {
  const { t } = useTranslation();
  const [startOpen, setStartOpen] = useState(false);
  const [endOpen, setEndOpen] = useState(false);

  const handleStartDateSelect = useCallback(
    (date: Date | undefined) => {
      if (date && endDate) {
        const end = parseDate(endDate);
        if (date > end) {
          toast.error(t('analytics.filter.startDateAfterEndError'));
          return;
        }
      }
      onStartChange(date || null);
      setStartOpen(false);
    },
    [endDate, onStartChange, t]
  );

  const handleEndDateSelect = useCallback(
    (date: Date | undefined) => {
      if (date && startDate) {
        const start = parseDate(startDate);
        if (date < start) {
          toast.error(t('analytics.filter.endDateBeforeStartError'));
          return;
        }
      }
      onEndChange(date || null);
      setEndOpen(false);
    },
    [startDate, onEndChange, t]
  );

  return (
    <div className='flex items-center gap-2'>
      <Popover open={startOpen} onOpenChange={setStartOpen}>
        <PopoverTrigger asChild>
          <Button
            variant='outline'
            className={cn('h-8 w-[130px] justify-start text-left text-xs font-normal', !startDate && 'text-muted-foreground')}
          >
            <IconCalendar className='mr-1 h-3 w-3' />
            {startDate || t('analytics.filter.startDate')}
          </Button>
        </PopoverTrigger>
        <PopoverContent className='w-auto p-0' align='start'>
          <Calendar mode='single' selected={startDate ? parseDate(startDate) : undefined} onSelect={handleStartDateSelect} initialFocus />
        </PopoverContent>
      </Popover>

      <span className='text-muted-foreground'>~</span>

      <Popover open={endOpen} onOpenChange={setEndOpen}>
        <PopoverTrigger asChild>
          <Button
            variant='outline'
            className={cn('h-8 w-[130px] justify-start text-left text-xs font-normal', !endDate && 'text-muted-foreground')}
          >
            <IconCalendar className='mr-1 h-3 w-3' />
            {endDate || t('analytics.filter.endDate')}
          </Button>
        </PopoverTrigger>
        <PopoverContent className='w-auto p-0' align='start'>
          <Calendar
            mode='single'
            selected={endDate ? parseDate(endDate) : undefined}
            onSelect={handleEndDateSelect}
            disabled={{ after: new Date() }}
            initialFocus
          />
        </PopoverContent>
      </Popover>
    </div>
  );
}

export interface AnalyticsDetailFilterOption {
  label: string;
  value: string;
}

interface AnalyticsDetailFilterBarProps {
  earliestDate?: string | null;
  timezone: string;
  startTime: string | null;
  endTime: string | null;
  selectedValues: string[];
  filterTitle: string;
  options: AnalyticsDetailFilterOption[];
  isLoadingOptions: boolean;
  onStartTimeChange: (time: string | null) => void;
  onEndTimeChange: (time: string | null) => void;
  onSelectedValuesChange: (values: string[]) => void;
  onReset: () => void;
}

interface APIKeyAnalyticsFilterBarProps {
  earliestDate?: string | null;
  timezone: string;
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

export function AnalyticsDetailFilterBar({
  earliestDate,
  timezone,
  startTime,
  endTime,
  selectedValues,
  filterTitle,
  options,
  isLoadingOptions,
  onStartTimeChange,
  onEndTimeChange,
  onSelectedValuesChange,
  onReset,
}: AnalyticsDetailFilterBarProps) {
  const { t } = useTranslation();

  const applyQuickRange = useCallback(
    (range: AnalyticsQuickRange) => {
      const dates = getAnalyticsQuickDateRange(
        range,
        timezone,
        earliestDate
      );
      onStartTimeChange(dates.startTime);
      onEndTimeChange(dates.endTime);
    },
    [earliestDate, onEndTimeChange, onStartTimeChange, timezone]
  );

  const hasFilters = Boolean(startTime || endTime || selectedValues.length > 0);

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
            onClick={() => applyQuickRange('today')}
          >
            {t('analytics.filter.today')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={() => applyQuickRange('yesterday')}
          >
            {t('analytics.filter.yesterday')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={() => applyQuickRange('thisWeek')}
          >
            {t('analytics.filter.thisWeek')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={() => applyQuickRange('thisMonth')}
          >
            {t('analytics.filter.thisMonth')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            className='h-8 text-xs'
            onClick={() => applyQuickRange('all')}
          >
            {t('analytics.filter.all')}
          </Button>
        </div>
      </div>

      <div className='flex flex-wrap items-center gap-2'>
        <AnalyticsFacetedFilter
          title={filterTitle}
          options={options}
          selectedValues={selectedValues}
          onSelectedValuesChange={onSelectedValuesChange}
          isLoading={isLoadingOptions}
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

export function APIKeyAnalyticsFilterBar({
  earliestDate,
  timezone,
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

  return (
    <AnalyticsDetailFilterBar
      earliestDate={earliestDate}
      timezone={timezone}
      startTime={startTime}
      endTime={endTime}
      selectedValues={templateIDs}
      filterTitle={t('analytics.apiKeyAnalytics.filter.template')}
      options={templates.map((template) => ({
        label: template.name,
        value: template.id,
      }))}
      isLoadingOptions={isLoadingTemplates}
      onStartTimeChange={onStartTimeChange}
      onEndTimeChange={onEndTimeChange}
      onSelectedValuesChange={onTemplateIDsChange}
      onReset={onReset}
    />
  );
}
