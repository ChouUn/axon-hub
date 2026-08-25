import assert from 'node:assert/strict';
import test from 'node:test';

import { getAnalyticsQuickDateRange } from './analytics/utils/date-range.ts';

const nearShanghaiMidnight = new Date('2026-08-25T16:30:00Z');

test('analytics quick ranges use the configured timezone date', () => {
  assert.deepEqual(
    getAnalyticsQuickDateRange(
      'today',
      'Asia/Shanghai',
      null,
      nearShanghaiMidnight
    ),
    { startTime: '2026-08-26', endTime: '2026-08-26' }
  );
  assert.deepEqual(
    getAnalyticsQuickDateRange(
      'today',
      'America/Los_Angeles',
      null,
      nearShanghaiMidnight
    ),
    { startTime: '2026-08-25', endTime: '2026-08-25' }
  );
});

test('analytics quick ranges calculate configured calendar boundaries', () => {
  assert.deepEqual(
    getAnalyticsQuickDateRange(
      'yesterday',
      'Asia/Shanghai',
      null,
      nearShanghaiMidnight
    ),
    { startTime: '2026-08-25', endTime: '2026-08-25' }
  );
  assert.deepEqual(
    getAnalyticsQuickDateRange(
      'thisWeek',
      'Asia/Shanghai',
      null,
      nearShanghaiMidnight
    ),
    { startTime: '2026-08-24', endTime: '2026-08-26' }
  );
  assert.deepEqual(
    getAnalyticsQuickDateRange(
      'thisMonth',
      'Asia/Shanghai',
      null,
      nearShanghaiMidnight
    ),
    { startTime: '2026-08-01', endTime: '2026-08-26' }
  );
});

test('analytics all range and invalid timezone use fallback semantics', () => {
  assert.deepEqual(
    getAnalyticsQuickDateRange(
      'all',
      'Asia/Shanghai',
      '2026-01-02',
      nearShanghaiMidnight
    ),
    { startTime: '2026-01-02', endTime: '2026-08-26' }
  );
  assert.deepEqual(
    getAnalyticsQuickDateRange(
      'today',
      'invalid/timezone',
      null,
      nearShanghaiMidnight
    ),
    { startTime: '2026-08-25', endTime: '2026-08-25' }
  );
});
