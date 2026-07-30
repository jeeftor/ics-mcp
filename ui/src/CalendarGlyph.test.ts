import { describe, expect, it } from 'vitest';
import { calendarColor, calendarColorPalette, nextPaletteColor } from './CalendarGlyph';

describe('calendarColor', () => {
  it('uses the persisted color and gives unstyled calendars a stable fallback', () => {
    expect(calendarColor({ key: 'WORK', color: '#ff0000' })).toBe('#ff0000');
    expect(calendarColor({ key: 'WORK' })).toBe(calendarColor({ key: 'WORK' }));
    expect(calendarColor({ key: 'WORK' })).not.toBe(calendarColor({ key: 'PERSONAL' }));
  });

  it('inherits a first tag color before using a stable palette fallback', () => {
    expect(calendarColor({ key: 'WORK', tags: ['Personal'] }, [{ name: 'Personal', color: '#3777b8' }])).toBe('#3777b8');
    expect(calendarColor({ key: 'WORK' })).toBe(calendarColor({ key: 'WORK' }));
  });

  it('chooses an unused curated color for new presentation defaults', () => {
    const first = nextPaletteColor([], 'Holiday');
    const second = nextPaletteColor([first], 'Personal');
    expect(calendarColorPalette).toContain(first);
    expect(calendarColorPalette).toContain(second);
    expect(second).not.toBe(first);
  });
});
