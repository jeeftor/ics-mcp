import { describe, expect, it } from 'vitest';
import { parseTags } from './api';

describe('parseTags', () => {
  it('trims, removes empty values, and deduplicates calendar tags', () => {
    expect(parseTags(' Work, Personal, Work, , School ')).toEqual(['Work', 'Personal', 'School']);
  });
});
