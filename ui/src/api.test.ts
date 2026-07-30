import { describe, expect, it } from 'vitest';
import { parseTags } from './api';
import { compactArguments, restEndpointInventory, restQuery, splitList } from './App';

describe('parseTags', () => {
  it('trims, removes empty values, and deduplicates calendar tags', () => {
    expect(parseTags(' Work, Personal, Work, , School ')).toEqual(['Work', 'Personal', 'School']);
  });
});

describe('API workspace controls', () => {
  it('omits blank meeting-preview arguments while preserving explicit false values', () => {
    expect(compactArguments({ timezone: '', fields: [], include_links: false, limit: 10 })).toEqual({ include_links: false, limit: 10 });
    expect(splitList(' when, calendar, , title ')).toEqual(['when', 'calendar', 'title']);
  });

  it('builds a shareable REST query from explorer controls and passthrough options', () => {
    expect(restQuery({ limit: '10', window: 'today', query: 'planning', fields: 'when, title', layout: 'agenda', time_style: '24h', show_timezone: true, extra: 'calendar_id=work' })).toBe('calendar_id=work&limit=10&window=today&query=planning&fields=when%2Ctitle&layout=agenda&time_style=24h&show_timezone=true');
  });

  it('keeps safe service reads and the read-only MCP bridge discoverable', () => {
    expect(restEndpointInventory.map(endpoint => endpoint.path)).toEqual(expect.arrayContaining([
      '/api/status', '/api/config', '/api/environment', '/api/calendars', '/api/tags', '/api/insights', '/api/rest/{tool_name}',
    ]));
  });
});
