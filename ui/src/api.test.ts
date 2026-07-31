import { describe, expect, it } from 'vitest';
import { parseTags } from './api';
import { compactArguments, pathForRoute, restEndpointInventory, restQuery, routeForLocation, splitList } from './App';

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

describe('admin-console routes', () => {
  const location = (pathname: string, search = '', hash = '') => ({ pathname, search, hash }) as Location;

  it('maps every addressable configuration and API tab to its view', () => {
    expect(routeForLocation(location('/config/runtime'))).toEqual({ page: 'config', configSection: 'settings' });
    expect(routeForLocation(location('/config/environment'))).toEqual({ page: 'config', configSection: 'environment' });
    expect(routeForLocation(location('/config/calendars'))).toEqual({ page: 'config', configSection: 'calendars' });
    expect(routeForLocation(location('/config/tags'))).toEqual({ page: 'config', configSection: 'tags' });
    expect(routeForLocation(location('/config/llm'))).toEqual({ page: 'ai' });
    expect(routeForLocation(location('/ai'))).toEqual({ page: 'ai' });
    expect(routeForLocation(location('/api/mcp-tools'))).toEqual({ page: 'api', apiSection: 'tools' });
    expect(routeForLocation(location('/api/meeting-preview'))).toEqual({ page: 'api', apiSection: 'meetings' });
    expect(routeForLocation(location('/api/rest-explorer'))).toEqual({ page: 'api', apiSection: 'rest' });
    expect(routeForLocation(location('/api/openapi'))).toEqual({ page: 'api', apiSection: 'openapi' });
  });

  it('keeps legacy locations useful and falls back safely for unknown paths', () => {
    expect(routeForLocation(location('/config'))).toEqual({ page: 'config', configSection: 'settings' });
    expect(routeForLocation(location('/api'))).toEqual({ page: 'api', apiSection: 'setup' });
    expect(routeForLocation(location('/', '', '#config/tags'))).toEqual({ page: 'config', configSection: 'tags' });
    expect(routeForLocation(location('/config/not-a-tab'))).toEqual({ page: 'config', configSection: 'settings' });
    expect(routeForLocation(location('/missing'))).toEqual({ page: 'calendar' });
  });

  it('generates canonical paths for navigation history', () => {
    expect(pathForRoute({ page: 'calendar' })).toBe('/');
    expect(pathForRoute({ page: 'ai' })).toBe('/ai');
    expect(pathForRoute({ page: 'api', apiSection: 'tools' })).toBe('/api/mcp-tools');
  });
});
