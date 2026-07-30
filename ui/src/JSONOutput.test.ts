import { describe, expect, it } from 'vitest';
import { formatJSON, jsonOutputValue } from './JSONOutput';

describe('JSON output formatting', () => {
  it('parses textual JSON and keeps original raw text', () => expect(jsonOutputValue('{"active":true,"count":2}')).toEqual({ kind: 'json', value: { active: true, count: 2 }, raw: '{"active":true,"count":2}' }));
  it('falls back to ordinary text', () => expect(jsonOutputValue('not JSON')).toEqual({ kind: 'text', raw: 'not JSON' }));
  it('supports compact and indented formats', () => { expect(formatJSON({ value: null }, true)).toBe('{"value":null}'); expect(formatJSON({ value: null }, false)).toBe('{\n  "value": null\n}'); });
});
