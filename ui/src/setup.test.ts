import { describe, expect, it } from 'vitest';
import { mcpClientConfig, setupBaseURL, setupEndpoints, updateCheckMessage } from './setup';

describe('Info & setup dashboard model', () => {
  it('uses the configured external URL for copyable MCP client configuration', () => {
    const baseURL = setupBaseURL('https://calendar.example.test/', 'http://localhost:3333');
    expect(baseURL).toBe('https://calendar.example.test');
    expect(JSON.parse(mcpClientConfig(baseURL))).toEqual({ mcpServers: { icsmcp: { url: 'https://calendar.example.test/mcp' } } });
  });

  it('keeps the primary setup and diagnostic endpoints discoverable', () => {
    expect(setupEndpoints.map(endpoint => endpoint.path)).toEqual(expect.arrayContaining([
      '/mcp', '/api/status', '/api/calendars', '/api/events', '/api/free-busy', '/openapi.json', '/docs',
    ]));
  });

  it('does not present a failed update check as current', () => {
    expect(updateCheckMessage({ enabled: true, outdated: false, current_version: 'v2.1.1', error: 'network unavailable' }))
      .toBe('Unable to check for updates: network unavailable');
  });
});
