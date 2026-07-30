import { describe, expect, it } from 'vitest';
import { calendarIconInheritLabel, noticeClassName } from './App';

describe('calendar administration polish', () => {
  it('names the icon fallback that will actually apply', () => {
    expect(calendarIconInheritLabel(['Work'])).toBe('Clear to inherit from tag');
    expect(calendarIconInheritLabel([])).toBe('Clear to inherit from default');
  });

  it('renders successful administrative feedback as a success notice', () => {
    expect(noticeClassName('success')).toBe('notice success');
    expect(noticeClassName('error')).toBe('notice error');
  });
});
