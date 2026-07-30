/** Values displayed by the Info & setup dashboard. */
export type SetupEndpoint = {
  label: string;
  path: string;
  description: string;
};

/** Returns a stable external base URL without a trailing slash. */
export function setupBaseURL(externalURL: string | undefined, browserOrigin: string): string {
  return (externalURL || browserOrigin).replace(/\/$/, '');
}

/** Builds the standard Streamable HTTP MCP client configuration. */
export function mcpClientConfig(baseURL: string): string {
  return JSON.stringify({ mcpServers: { icsmcp: { url: `${baseURL}/mcp` } } }, null, 2);
}

/** Public endpoints useful to an administrator during setup and diagnostics. */
export const setupEndpoints: SetupEndpoint[] = [
  { label: 'MCP endpoint', path: '/mcp', description: 'Streamable HTTP endpoint for MCP clients.' },
  { label: 'Service status', path: '/api/status', description: 'Version, timezone, and configured calendar summary.' },
  { label: 'Calendar administration', path: '/api/calendars', description: 'Calendar inventory and refresh state.' },
  { label: 'REST events', path: '/api/events', description: 'Events in JSON or a selected text format.' },
  { label: 'Free/busy', path: '/api/free-busy', description: 'Availability blocks for scheduling.' },
  { label: 'OpenAPI document', path: '/openapi.json', description: 'Complete REST route inventory.' },
  { label: 'REST documentation', path: '/docs', description: 'Human-readable REST documentation entry point.' },
];

/** Formats a check result without making a missing or failed check look current. */
export function updateCheckMessage(update?: { enabled: boolean; outdated: boolean; latest_version?: string; current_version: string; error?: string }): string {
  if (!update) return 'Checking for updates…';
  if (!update.enabled) return update.error || 'Update checks are disabled.';
  if (update.error) return `Unable to check for updates: ${update.error}`;
  if (update.outdated) return `Update available: ${update.latest_version || 'a newer release'}`;
  return `Up to date (${update.current_version})`;
}
