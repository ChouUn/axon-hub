import assert from 'node:assert/strict';
import test from 'node:test';
import {
  DEFAULT_MODEL_ANALYTICS_SORT,
  getNextModelAnalyticsSort,
  sortModelAnalyticsStats,
} from './analytics/utils/model-analytics-sort.ts';

function stat(id, overrides = {}) {
  return {
    id,
    name: id,
    requestCount: 0,
    totalTokens: 0,
    cost: 0,
    costPerMillion: 0,
    successRate: 0,
    cacheHitRate: 0,
    avgFirstTokenLatencyMs: null,
    avgOutputTokensPerSecond: null,
    ...overrides,
  };
}

const data = [
  {
    ...stat('model-a', { requestCount: 2, totalTokens: 100, cost: 1 }),
    channels: [stat('channel-a1', { requestCount: 1, cost: 0.25 }), stat('channel-a2', { requestCount: 3, cost: 0.75 })],
  },
  {
    ...stat('model-b', { requestCount: 4, totalTokens: 50, cost: 2 }),
    channels: [stat('channel-b1', { requestCount: 4, cost: 1.5 }), stat('channel-b2', { requestCount: 2, cost: 0.5 })],
  },
];

test('sort state cycles through descending, ascending, then cost fallback', () => {
  const requestDescending = getNextModelAnalyticsSort(DEFAULT_MODEL_ANALYTICS_SORT, 'requestCount');
  assert.deepEqual(requestDescending, {
    key: 'requestCount',
    direction: 'desc',
  });

  const requestAscending = getNextModelAnalyticsSort(requestDescending, 'requestCount');
  assert.deepEqual(requestAscending, {
    key: 'requestCount',
    direction: 'asc',
  });
  assert.deepEqual(getNextModelAnalyticsSort(requestAscending, 'requestCount'), DEFAULT_MODEL_ANALYTICS_SORT);

  const costAscending = getNextModelAnalyticsSort(DEFAULT_MODEL_ANALYTICS_SORT, 'cost');
  assert.deepEqual(costAscending, { key: 'cost', direction: 'asc' });
  assert.deepEqual(getNextModelAnalyticsSort(costAscending, 'cost'), DEFAULT_MODEL_ANALYTICS_SORT);
});

test('selected column sorts models and each model channel list', () => {
  const sorted = sortModelAnalyticsStats(data, {
    key: 'requestCount',
    direction: 'asc',
  });

  assert.deepEqual(
    sorted.map((model) => model.id),
    ['model-a', 'model-b']
  );
  assert.deepEqual(
    sorted[0].channels.map((channel) => channel.id),
    ['channel-a1', 'channel-a2']
  );
  assert.deepEqual(
    sorted[1].channels.map((channel) => channel.id),
    ['channel-b2', 'channel-b1']
  );
});

test('cache hit rate sorts models and each model channel list', () => {
  const cacheData = [
    {
      ...stat('model-a', { cacheHitRate: 25 }),
      channels: [stat('channel-a1', { cacheHitRate: 75 }), stat('channel-a2', { cacheHitRate: 10 })],
    },
    {
      ...stat('model-b', { cacheHitRate: 50 }),
      channels: [stat('channel-b1', { cacheHitRate: 20 }), stat('channel-b2', { cacheHitRate: 80 })],
    },
  ];

  const sorted = sortModelAnalyticsStats(cacheData, {
    key: 'cacheHitRate',
    direction: 'desc',
  });

  assert.deepEqual(
    sorted.map((model) => model.id),
    ['model-b', 'model-a']
  );
  assert.deepEqual(
    sorted[0].channels.map((channel) => channel.id),
    ['channel-b2', 'channel-b1']
  );
  assert.deepEqual(
    sorted[1].channels.map((channel) => channel.id),
    ['channel-a1', 'channel-a2']
  );
});

test('sorting preserves source order and stable model identities', () => {
  const expandedModels = new Set(['model-a']);
  const sorted = sortModelAnalyticsStats(data, DEFAULT_MODEL_ANALYTICS_SORT);

  assert.deepEqual(
    sorted.map((model) => model.id),
    ['model-b', 'model-a']
  );
  assert.deepEqual(
    data.map((model) => model.id),
    ['model-a', 'model-b']
  );
  assert.deepEqual(
    data[0].channels.map((channel) => channel.id),
    ['channel-a1', 'channel-a2']
  );
  assert.equal(expandedModels.has(sorted[1].id), true);
});

test('missing performance metrics remain last in both directions', () => {
  const metrics = [
    {
      ...stat('missing'),
      channels: [],
    },
    {
      ...stat('measured', { avgFirstTokenLatencyMs: 120 }),
      channels: [],
    },
  ];

  for (const direction of ['asc', 'desc']) {
    const sorted = sortModelAnalyticsStats(metrics, {
      key: 'avgFirstTokenLatencyMs',
      direction,
    });
    assert.deepEqual(
      sorted.map((model) => model.id),
      ['measured', 'missing']
    );
  }
});
