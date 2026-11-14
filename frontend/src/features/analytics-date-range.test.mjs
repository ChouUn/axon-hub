import assert from 'node:assert/strict';
import test from 'node:test';

import {
  analyticsDateRangeToTimestamps,
  getAnalyticsCalendarDate,
  getAnalyticsQuickDateRange,
} from './analytics/utils/date-range.ts';

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

test('analytics UTC calendar days become inclusive RFC3339 bounds', () => {
  assert.deepEqual(
    analyticsDateRangeToTimestamps(
      { startTime: '2026-03-10', endTime: '2026-03-10' },
      'UTC'
    ),
    {
      startTime: '2026-03-10T00:00:00.000Z',
      endTime: '2026-03-10T23:59:59.999Z',
    }
  );
});

test('analytics Shanghai calendar days preserve the UTC offset', () => {
  assert.deepEqual(
    analyticsDateRangeToTimestamps(
      { startTime: '2026-03-10', endTime: '2026-03-10' },
      'Asia/Shanghai'
    ),
    {
      startTime: '2026-03-09T16:00:00.000Z',
      endTime: '2026-03-10T15:59:59.999Z',
    }
  );
});

test('analytics spring-forward calendar days span 23 hours', () => {
  assert.deepEqual(
    analyticsDateRangeToTimestamps(
      { startTime: '2026-03-08', endTime: '2026-03-08' },
      'America/New_York'
    ),
    {
      startTime: '2026-03-08T05:00:00.000Z',
      endTime: '2026-03-09T03:59:59.999Z',
    }
  );
});

test('analytics fall-back calendar days span 25 hours', () => {
  assert.deepEqual(
    analyticsDateRangeToTimestamps(
      { startTime: '2026-11-01', endTime: '2026-11-01' },
      'America/New_York'
    ),
    {
      startTime: '2026-11-01T04:00:00.000Z',
      endTime: '2026-11-02T04:59:59.999Z',
    }
  );
});

test('analytics calendar days start at the first instant after a midnight DST jump', () => {
  assert.deepEqual(
    analyticsDateRangeToTimestamps(
      { startTime: '2018-11-04', endTime: '2018-11-04' },
      'America/Sao_Paulo'
    ),
    {
      startTime: '2018-11-04T03:00:00.000Z',
      endTime: '2018-11-05T01:59:59.999Z',
    }
  );
});

test('analytics calendar ranges preserve missing bounds independently', () => {
  assert.deepEqual(
    analyticsDateRangeToTimestamps(
      { startTime: null, endTime: null },
      'Asia/Shanghai'
    ),
    { startTime: null, endTime: null }
  );
  assert.deepEqual(
    analyticsDateRangeToTimestamps(
      { startTime: null, endTime: '2026-03-10' },
      'Asia/Shanghai'
    ),
    { startTime: null, endTime: '2026-03-10T15:59:59.999Z' }
  );
  assert.deepEqual(
    analyticsDateRangeToTimestamps(
      { startTime: '2026-03-10', endTime: null },
      'Asia/Shanghai'
    ),
    { startTime: '2026-03-09T16:00:00.000Z', endTime: null }
  );
});

test('analytics timestamps resolve to the configured timezone calendar date', () => {
  assert.equal(
    getAnalyticsCalendarDate(nearShanghaiMidnight, 'Asia/Shanghai'),
    '2026-08-26'
  );
  assert.equal(
    getAnalyticsCalendarDate(nearShanghaiMidnight, 'America/Los_Angeles'),
    '2026-08-25'
  );
});
