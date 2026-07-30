import { describe, expect, it, vi } from 'vitest';
import { Calendar } from './api';
import { calendarIconInheritLabel, noticeClassName, openCalendarAppearance, supportsScreenColorPicker } from './App';

describe('calendar administration polish', () => {
  it('names the icon fallback that will actually apply', () => {
    expect(calendarIconInheritLabel(['Work'])).toBe('Clear to inherit from tag');
    expect(calendarIconInheritLabel([])).toBe('Clear to inherit from default');
  });

  it('renders successful administrative feedback as a success notice', () => {
    expect(noticeClassName('success')).toBe('notice success');
    expect(noticeClassName('error')).toBe('notice error');
  });

  it('opens the appearance editor from the palette action without refreshing', () => {
    const calendar: Calendar = { id: 'work', key: 'WORK', name: 'Work', url: 'https://example.com/work.ics', tags: [], enabled: true, include_in_general_queries: true, event_count: 0 };
    const openEditor = vi.fn();
    const refresh = vi.fn();

    openCalendarAppearance(calendar, openEditor);

    expect(openEditor).toHaveBeenCalledOnce();
    expect(openEditor).toHaveBeenCalledWith(calendar);
    expect(refresh).not.toHaveBeenCalled();
  });

  it('only enables the screen color picker when the browser supports it', () => {
    expect(supportsScreenColorPicker({})).toBe(false);
    expect(supportsScreenColorPicker({ EyeDropper: class { open = async () => ({ sRGBHex: '#123456' }); } })).toBe(true);
  });
});
