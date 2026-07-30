import { describe, expect, it } from 'vitest';
import { refreshIntervalChoice } from './RefreshIntervalPicker';

describe('refreshIntervalChoice', () => {
  it('recognizes concise and Go-formatted quick choices', () => {
    expect(refreshIntervalChoice('5m')).toBe('5m');
    expect(refreshIntervalChoice('5m0s')).toBe('5m');
    expect(refreshIntervalChoice('1h0m0s')).toBe('1h');
  });

  it('keeps arbitrary and blank durations on the Custom path', () => {
    expect(refreshIntervalChoice('45m')).toBeUndefined();
    expect(refreshIntervalChoice('')).toBeUndefined();
    expect(refreshIntervalChoice('not-a-duration')).toBeUndefined();
  });
});
