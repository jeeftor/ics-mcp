import { describe, expect, it } from 'vitest';
import { allDayRows, meetingMinutes, placeTimedMeetings } from './planner';
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

  it('keeps all-day overflow compact', () => {
    const rows = allDayRows([event('00:00', '00:30'), event('00:00', '00:30'), event('00:00', '00:30'), event('00:00', '00:30')]);
    expect(rows.visible).toHaveLength(3);
    expect(rows.overflow).toBe(1);
  });
});
