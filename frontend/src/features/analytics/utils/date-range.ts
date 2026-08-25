export type AnalyticsQuickRange =
  | 'today'
  | 'yesterday'
  | 'thisWeek'
  | 'thisMonth'
  | 'all';

export interface AnalyticsDateRange {
  startTime: string | null;
  endTime: string | null;
}

function calendarDateInTimezone(date: Date, timezone: string): Date {
  let formatter: Intl.DateTimeFormat;
  try {
    formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: timezone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
  } catch {
    formatter = new Intl.DateTimeFormat('en-US', {
      timeZone: 'UTC',
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    });
  }

  const parts = Object.fromEntries(
    formatter
      .formatToParts(date)
      .filter((part) => part.type !== 'literal')
      .map((part) => [part.type, part.value])
  );
  return new Date(
    Date.UTC(Number(parts.year), Number(parts.month) - 1, Number(parts.day))
  );
}

function formatCalendarDate(date: Date): string {
  const year = date.getUTCFullYear();
  const month = String(date.getUTCMonth() + 1).padStart(2, '0');
  const day = String(date.getUTCDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function getAnalyticsQuickDateRange(
  range: AnalyticsQuickRange,
  timezone: string,
  earliestDate: string | null = null,
  now: Date = new Date()
): AnalyticsDateRange {
  const today = calendarDateInTimezone(now, timezone);
  const endTime = formatCalendarDate(today);

  if (range === 'all') {
    return {
      startTime: earliestDate,
      endTime: earliestDate ? endTime : null,
    };
  }

  const start = new Date(today);
  switch (range) {
    case 'yesterday':
      start.setUTCDate(start.getUTCDate() - 1);
      return {
        startTime: formatCalendarDate(start),
        endTime: formatCalendarDate(start),
      };
    case 'thisWeek': {
      const daysSinceMonday = (start.getUTCDay() + 6) % 7;
      start.setUTCDate(start.getUTCDate() - daysSinceMonday);
      break;
    }
    case 'thisMonth':
      start.setUTCDate(1);
      break;
    case 'today':
      break;
  }

  return {
    startTime: formatCalendarDate(start),
    endTime,
  };
}
