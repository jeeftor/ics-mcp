import { describe, expect, it } from 'vitest';
import { addDays, dateKey, dateRangeLabel, meetingDate, meetingTime, plannerViewAnchor, startOfWeek, weekRangeLabel } from './calendarUtils';

describe('calendar planner date helpers', () => {
  it('anchors every planner range on Monday at local midnight', () => {
    const start = startOfWeek(new Date(2026, 6, 30, 15, 45));

    expect(start.getDay()).toBe(1);
    expect(start.getHours()).toBe(0);
    expect(start.getMinutes()).toBe(0);
    expect(dateKey(start)).toBe('2026-07-27');
  });

  it('creates a seven-day inclusive planner range without mutating its start', () => {
    const start = new Date(2026, 11, 28, 0, 0);
    const end = addDays(start, 6);

    expect(dateKey(start)).toBe('2026-12-28');
    expect(dateKey(end)).toBe('2027-01-03');
    expect(weekRangeLabel(start)).toBe('Dec 28 – Jan 3, 2027');
  });

  it('uses a compact label for short planner ranges', () => {
    expect(dateRangeLabel(new Date(2026, 7, 6), 1)).toBe('Aug 6');
    expect(dateRangeLabel(new Date(2026, 7, 6), 3)).toBe('Aug 6 – Aug 8');
    expect(dateRangeLabel(new Date(2026, 7, 30), 3)).toBe('Aug 30 – Sep 1');
  });

  it('anchors rolling planner views on today', () => {
    const today = new Date(2026, 6, 30, 15, 45);

    expect(dateKey(plannerViewAnchor(1, today))).toBe('2026-07-30');
    expect(dateKey(plannerViewAnchor(3, today))).toBe('2026-07-30');
    expect(dateKey(plannerViewAnchor(7, today))).toBe('2026-07-30');
  });

  it('keeps all-day events separate from timed event labels', () => {
    expect(meetingDate({ name: 'Day off', date: '2026-07-30', all_day: true })).toBe('2026-07-30');
    expect(meetingTime({ name: 'Day off', all_day: true })).toBe('All day');
    expect(meetingTime({ name: 'Standup', start: '09:30', end: '10:00' })).toBe('9:30 AM – 10:00 AM');
    expect(meetingTime({ name: 'Standup', start: '09:30', end: '10:00' }, '24h')).toBe('09:30 – 10:00');
  });
});
