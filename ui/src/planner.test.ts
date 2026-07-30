import { describe, expect, it } from 'vitest';
import { allDayRows, meetingMinutes, meetingsForVisibleCalendars, placeAllDayMeetings, placeTimedMeetings, timedEventTitleLines, visibleAllDayPlacements } from './planner';
import { Meeting } from './api';

const event = (start: string, end: string): Meeting => ({ name: start, date: '2026-07-30', start, end });

describe('planner placement', () => {
  it('uses minute offsets and a useful default duration', () => {
    expect(meetingMinutes(event('09:30', '10:45'))).toEqual({ start: 570, end: 645 });
    expect(meetingMinutes({ name: 'Untimed', start: 'bad' })).toBeUndefined();
    expect(meetingMinutes({ name: 'Default', start: '09:30' })).toEqual({ start: 570, end: 600 });
  });

  it('gives concurrent events distinct equal-width lanes', () => {
    const placements = placeTimedMeetings([event('09:00', '10:00'), event('09:30', '10:30'), event('11:00', '12:00')]);
    expect(placements.map(item => [item.lane, item.lanes])).toEqual([[0, 2], [1, 2], [0, 1]]);
  });

  it('wraps timed titles only when the card has room for them', () => {
    expect(timedEventTitleLines(30, false)).toBe(1);
    expect(timedEventTitleLines(44, false)).toBe(2);
    expect(timedEventTitleLines(60, true)).toBe(1);
    expect(timedEventTitleLines(68, true)).toBe(2);
  });

  it('keeps all-day overflow compact', () => {
    const rows = allDayRows([event('00:00', '00:30'), event('00:00', '00:30'), event('00:00', '00:30'), event('00:00', '00:30')]);
    expect(rows.visible).toHaveLength(3);
    expect(rows.overflow).toBe(1);
  });

  it('clips multi-day all-day events to the visible range and gives overlaps separate rows', () => {
    const meetings: Meeting[] = [
      { name: 'Camping', date: '2026-07-28', end_date: '2026-08-01', all_day: true },
      { name: 'One day', date: '2026-07-30', end_date: '2026-07-31', all_day: true },
      { name: 'After range', date: '2026-08-03', end_date: '2026-08-04', all_day: true },
    ];
    expect(placeAllDayMeetings(meetings, '2026-07-29', 3).map(item => [item.meeting.name, item.start, item.span, item.row])).toEqual([
      ['Camping', 0, 3, 0],
      ['One day', 1, 1, 1],
    ]);
  });

  it('treats omitted and same-day all-day end dates as one day', () => {
    const meetings: Meeting[] = [
      { name: 'No end', date: '2026-07-30', all_day: true },
      { name: 'Same day', date: '2026-07-31', end_date: '2026-07-31', all_day: true },
    ];
    expect(placeAllDayMeetings(meetings, '2026-07-30', 3).map(item => [item.start, item.span])).toEqual([[0, 1], [1, 1]]);
  });

  it('shows three all-day rows before counting overflow, then expands only the chosen day', () => {
    const placements = placeAllDayMeetings([
      { name: 'One', date: '2026-07-30', all_day: true },
      { name: 'Two', date: '2026-07-30', all_day: true },
      { name: 'Three', date: '2026-07-30', all_day: true },
      { name: 'Four', date: '2026-07-30', all_day: true },
      { name: 'Friday', date: '2026-07-31', all_day: true },
    ], '2026-07-30', 2);
    const compact = visibleAllDayPlacements(placements, 2, 3, new Set());
    expect(compact.overflowByDay).toEqual([1, 0]);
    expect(compact.visible).toHaveLength(4);
    expect(compact.visible.map(item => item.row)).not.toContain(3);
    expect(compact.visible.map(item => item.meeting.name)).not.toContain('Two');

    const expanded = visibleAllDayPlacements(placements, 2, 3, new Set([0]));
    expect(expanded.visible.map(item => item.meeting.name)).toContain('Two');
    expect(expanded.rows).toBe(4);
  });

  it('filters an already fetched range locally when a calendar is hidden', () => {
    const meetings: Meeting[] = [{ name: 'Visible', calendar_id: 'one' }, { name: 'Hidden', calendar_id: 'two' }];

    expect(meetingsForVisibleCalendars(meetings, ['one']).map((meeting) => meeting.name)).toEqual(['Visible']);
    expect(meetings).toHaveLength(2);
  });
});
