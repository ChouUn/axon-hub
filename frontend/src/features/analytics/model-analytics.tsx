import { useMemo, useState } from 'react';
import { useNavigate } from '@tanstack/react-router';
import { ArrowLeft } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Header } from '@/components/layout/header';
import { Button } from '@/components/ui/button';
import { cn } from '@/lib/utils';
import { useGeneralSettings } from '@/features/system/data/system';
import {
  useAnalyticsChannelTags,
  useAnalyticsModelStats,
  type AnalyticsModelFilter,
} from './data/analytics';
import { AnalyticsDetailFilterBar } from './components/api-key-analytics-filter-bar';
import { ModelAnalyticsTable } from './components/model-analytics-table';

export default function ModelAnalyticsPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [startTime, setStartTime] = useState<string | null>(null);
  const [endTime, setEndTime] = useState<string | null>(null);
  const [channelTags, setChannelTags] = useState<string[]>([]);
  const { data: generalSettings } = useGeneralSettings();
  const {
    data: availableTags = [],
    isLoading: isTagsLoading,
    error: tagsError,
  } = useAnalyticsChannelTags();
  const filter = useMemo<AnalyticsModelFilter>(
    () => ({
      startTime,
      endTime,
      channelTags: channelTags.length > 0 ? channelTags : undefined,
    }),
    [channelTags, endTime, startTime]
  );
  const {
    data: stats = [],
    isLoading: isStatsLoading,
    error: statsError,
  } = useAnalyticsModelStats(filter);

  const handleReset = () => {
    setStartTime(null);
    setEndTime(null);
    setChannelTags([]);
  };

  const error = tagsError || statsError;
  const currencyCode = generalSettings?.currencyCode || 'USD';

  return (
    <div className='flex-1 space-y-6 p-8 pt-6'>
      <Header>
        <h1 className='text-2xl font-bold tracking-tight'>
          {t('analytics.modelAnalytics.title')}
        </h1>
      </Header>

      <Button onClick={() => navigate({ to: '/' })} variant='outline'>
        <ArrowLeft className='h-4 w-4' />
        {t('dashboard.channelSuccessRates.backToDashboard')}
      </Button>

      <p className='text-sm text-muted-foreground'>
        {t('analytics.modelAnalytics.description')}
      </p>

      <AnalyticsDetailFilterBar
        timezone={generalSettings?.timezone || 'UTC'}
        startTime={startTime}
        endTime={endTime}
        selectedValues={channelTags}
        filterTitle={t('analytics.modelAnalytics.filter.channelTag')}
        options={availableTags.map((tag) => ({ label: tag, value: tag }))}
        isLoadingOptions={isTagsLoading}
        onStartTimeChange={setStartTime}
        onEndTimeChange={setEndTime}
        onSelectedValuesChange={setChannelTags}
        onReset={handleReset}
      />

      {error ? (
        <div
          className={cn(
            'rounded-lg border border-destructive/40 bg-destructive/5',
            'p-4 text-sm text-destructive'
          )}
        >
          {t('common.loadError')} {error.message}
        </div>
      ) : (
        <ModelAnalyticsTable
          data={stats}
          isLoading={isStatsLoading}
          currencyCode={currencyCode}
        />
      )}
    </div>
  );
}
