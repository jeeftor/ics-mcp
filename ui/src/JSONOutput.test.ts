import { describe, expect, it } from 'vitest';
import { formatJSON, jsonOutputValue } from './JSONOutput';

describe('JSON output formatting', () => {
  it('parses textual JSON and keeps original raw text', () => expect(jsonOutputValue('{"active":true,"count":2}')).toEqual({ kind: 'json', value: { active: true, count: 2 }, raw: '{"active":true,"count":2}' }));
  it('falls back to ordinary text', () => expect(jsonOutputValue('not JSON')).toEqual({ kind: 'text', raw: 'not JSON' }));
  it('supports compact and indented formats', () => { expect(formatJSON({ value: null }, true)).toBe('{"value":null}'); expect(formatJSON({ value: null }, false)).toBe('{\n  "value": null\n}'); });
  it('detects HTML and tags it as html', () => {
    expect(jsonOutputValue('<div>Hello</div>')).toEqual({ kind: 'html', raw: '<div>Hello</div>' });
    expect(jsonOutputValue('<p class="x">Text</p>')).toEqual({ kind: 'html', raw: '<p class="x">Text</p>' });
  });
  it('does not misclassify plain text starting with < as HTML without tags', () => {
    expect(jsonOutputValue('less than 5 items')).toEqual({ kind: 'text', raw: 'less than 5 items' });
    expect(jsonOutputValue('<3')).toEqual({ kind: 'text', raw: '<3' });
  });
});
