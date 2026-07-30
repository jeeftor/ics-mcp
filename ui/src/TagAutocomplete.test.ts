import { describe, expect, it } from 'vitest';
import { activeTagFragment, matchingTags, replaceActiveTag } from './TagAutocomplete';

describe('calendar tag autocomplete', () => {
  const tags = [{ name: 'School' }, { name: 'Work' }, { name: 'Personal' }];

  it('suggests the active comma-separated fragment and replaces only that fragment', () => {
    expect(matchingTags('Work, Sch, Personal', 9, tags)).toEqual([{ name: 'School' }]);
    expect(replaceActiveTag('Work, Sch, Personal', 9, 'School')).toEqual({ value: 'Work, School, Personal', cursor: 12 });
  });

  it('retains whitespace and cursor scope for the current tag', () => {
    expect(activeTagFragment('Work,  Sch', 10)).toEqual({ start: 5, end: 10, query: 'Sch' });
    expect(replaceActiveTag('Work,  Sch', 10, 'School')).toEqual({ value: 'Work,  School', cursor: 13 });
  });
});
