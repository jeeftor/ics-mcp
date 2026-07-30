import { describe, expect, it } from 'vitest';
import { calendarColor } from './CalendarGlyph';

describe('calendarColor', () => {
  it('uses the persisted color and gives unstyled calendars a stable fallback', () => {
    expect(calendarColor({ key: 'WORK', color: '#ff0000' })).toBe('#ff0000');
    expect(calendarColor({ key: 'WORK' })).toBe(calendarColor({ key: 'WORK' }));
    expect(calendarColor({ key: 'WORK' })).not.toBe(calendarColor({ key: 'PERSONAL' }));
  });
});
