import type { AnalyticsModelChannelStat, AnalyticsModelStat } from '../data/analytics';

export type ModelAnalyticsSortKey =
  | 'name'
  | 'requestCount'
  | 'totalTokens'
  | 'costPerMillion'
  | 'successRate'
  | 'cacheHitRate'
  | 'avgFirstTokenLatencyMs'
  | 'avgOutputTokensPerSecond'
  | 'cost';

export type ModelAnalyticsSortDirection = 'asc' | 'desc';

export interface ModelAnalyticsSort {
  key: ModelAnalyticsSortKey;
  direction: ModelAnalyticsSortDirection;
}

export const DEFAULT_MODEL_ANALYTICS_SORT: ModelAnalyticsSort = {
  key: 'cost',
  direction: 'desc',
};

type SortableModelAnalyticsStat = Pick<AnalyticsModelChannelStat, ModelAnalyticsSortKey>;

export function getModelAnalyticsSortDirection(sort: ModelAnalyticsSort, key: ModelAnalyticsSortKey): ModelAnalyticsSortDirection | null {
  return sort.key === key ? sort.direction : null;
}

export function getNextModelAnalyticsSort(sort: ModelAnalyticsSort, key: ModelAnalyticsSortKey): ModelAnalyticsSort {
  if (sort.key !== key) {
    return { key, direction: 'desc' };
  }
  if (sort.direction === 'desc') {
    return { key, direction: 'asc' };
  }
  return DEFAULT_MODEL_ANALYTICS_SORT;
}

function compareModelAnalyticsStats(left: SortableModelAnalyticsStat, right: SortableModelAnalyticsStat, sort: ModelAnalyticsSort): number {
  const leftValue = left[sort.key];
  const rightValue = right[sort.key];

  if (leftValue == null && rightValue == null) return 0;
  if (leftValue == null) return 1;
  if (rightValue == null) return -1;

  const comparison =
    typeof leftValue === 'string' && typeof rightValue === 'string'
      ? leftValue.localeCompare(rightValue)
      : Number(leftValue) - Number(rightValue);

  return sort.direction === 'asc' ? comparison : -comparison;
}

function sortStats<T extends SortableModelAnalyticsStat>(stats: readonly T[], sort: ModelAnalyticsSort): T[] {
  return [...stats].sort((left, right) => compareModelAnalyticsStats(left, right, sort));
}

export function sortModelAnalyticsStats(data: readonly AnalyticsModelStat[], sort: ModelAnalyticsSort): AnalyticsModelStat[] {
  return sortStats(data, sort).map((model) => ({
    ...model,
    channels: sortStats(model.channels, sort),
  }));
}
