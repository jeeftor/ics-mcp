export type Calendar = {
  id: string; key: string; name: string; url: string; tags: string[]; enabled: boolean;
  include_in_general_queries: boolean; event_count: number; last_success?: string;
  next_refresh?: string; last_error?: string;
};

export type Status = {
  timezone: string; external_url?: string; calendars: Calendar[];
  version: { version: string; commit: string; date: string };
};

export type RuntimeConfig = {
  refresh_interval: string; timezone: string; external_url: string;
  update_check: boolean; sources: Record<string, string>;
};

export type Tag = { name: string; calendar_count: number };
export type Meeting = { name: string; when?: string; start_time?: string; end_time?: string; calendar_name?: string; ongoing?: boolean; all_day?: boolean; meeting_url?: string };
export type Tool = { name: string; description: string; category: string; read_only: boolean; destructive: boolean; default_arguments: Record<string, unknown> };

export function parseTags(value: string): string[] {
  return [...new Set(value.split(',').map((tag) => tag.trim()).filter(Boolean))];
}

const tokenKey = 'icsmcp.auth-token';

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json');
  const token = sessionStorage.getItem(tokenKey);
  if (token) headers.set('Authorization', `Bearer ${token}`);
  const response = await fetch(path, { ...init, headers });
  if (response.status === 401 && !token) {
    const supplied = window.prompt('Enter the ICS MCP bearer token');
    if (supplied) {
      sessionStorage.setItem(tokenKey, supplied.trim());
      return api<T>(path, init);
    }
  }
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || response.statusText);
  }
  return response.json() as Promise<T>;
}

export const calendarAPI = {
  list: () => api<Calendar[]>('/api/calendars'),
  tags: () => api<Tag[]>('/api/tags'),
  add: (body: { name: string; url: string; tags: string[] }) => api<Calendar>('/api/calendars', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: Partial<Pick<Calendar, 'name' | 'tags' | 'enabled' | 'include_in_general_queries'>>) => api<Calendar>(`/api/calendars/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  remove: (id: string) => api<void>(`/api/calendars/${id}`, { method: 'DELETE' }),
  refresh: (id: string) => api<void>(`/api/calendars/${id}/refresh`, { method: 'POST' }),
  refreshAll: () => api<unknown>('/api/rest/refresh_all_calendars', { method: 'POST', body: '{}' })
};
