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

function calendarDateFormatter(timezone: string): Intl.DateTimeFormat {
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

  return formatter;
}

function calendarDateInTimezone(date: Date, formatter: Intl.DateTimeFormat): Date {
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
  const today = calendarDateInTimezone(now, calendarDateFormatter(timezone));
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

export function getAnalyticsCalendarDate(date: Date, timezone: string): string {
  return formatCalendarDate(calendarDateInTimezone(date, calendarDateFormatter(timezone)));
}

export function analyticsDateRangeToTimestamps(range: AnalyticsDateRange, timezone: string): AnalyticsDateRange {
  const formatter = calendarDateFormatter(timezone);
  const startOfCalendarDay = (day: string, dayOffset: number): number => {
    const [year, month, date] = day.split('-').map(Number);
    const target = Date.UTC(year, month - 1, date + dayOffset);
    let lower = target - 86_400_000;
    let upper = target + 86_400_000;

    // Find the first instant of the calendar day, including DST changes at midnight.
    // Searching each boundary independently avoids assuming that a day lasts 24 hours.
    while (lower < upper) {
      const middle = Math.floor((lower + upper) / 2);
      if (calendarDateInTimezone(new Date(middle), formatter).getTime() < target) lower = middle + 1;
      else upper = middle;
    }
    return lower;
  };

  return {
    startTime: range.startTime ? new Date(startOfCalendarDay(range.startTime, 0)).toISOString() : null,
    endTime: range.endTime ? new Date(startOfCalendarDay(range.endTime, 1) - 1).toISOString() : null,
  };
}
