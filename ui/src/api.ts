export type Calendar = {
  id: string; key: string; name: string; url: string; color?: string; icon?: string; custom_icon_url?: string; refresh_interval?: string; tags: string[]; enabled: boolean;
  include_in_general_queries: boolean; event_count: number; last_success?: string;
  last_attempt?: string; next_refresh?: string; last_error?: string;
};

export type FeedValidation = { ok: boolean; status_code?: number; event_count: number; meetings?: Meeting[]; error?: string };

export type Status = {
  timezone: string; external_url?: string; calendars: Calendar[];
  version: { version: string; commit: string; date: string };
};

export type RuntimeConfig = {
  refresh_interval: string; timezone: string; external_url: string;
  update_check: boolean; sources: Record<string, string>;
};
/** Environment values are safe to render; sensitive entries are always redacted by the server. */
export type EnvironmentVariable = { name: string; value: string; present: boolean; source: string; sensitive: boolean };

export type Tag = { name: string; calendar_count: number; color?: string; icon?: string; refresh_interval?: string; position?: number };
export type Meeting = { name: string; title?: string; when?: string; date?: string; end_date?: string; start?: string; end?: string; timezone?: string; duration_minutes?: number; description?: string; calendar_id?: string; calendar_name?: string; calendar?: string; ongoing?: boolean; all_day?: boolean; cancelled?: boolean; recurring?: boolean; meeting_url?: string; attendance_status?: 'accepted' | 'tentative' | 'declined' | 'needs-action' };
export type Tool = { name: string; description: string; category: string; read_only: boolean; destructive: boolean; default_arguments: Record<string, unknown> };
export type ToolCallResponse = { tool: string; result: unknown };
export type UpdateCheck = { enabled: boolean; current_version: string; latest_version?: string; outdated: boolean; release_url?: string; checked_at?: string; error?: string };
/** A profile is always redacted: the browser never receives the bearer key. */
export type LLMBackend = 'openai' | 'ollama' | 'lemonade';
export type LLMProfile = { enabled: boolean; backend: LLMBackend; endpoint: string; model: string; api_key_configured: boolean; source: string };
export type LLMProfileInput = { enabled?: boolean; backend?: LLMBackend; endpoint?: string; model?: string; api_key?: string };
export type LLMConnectionInput = { backend: LLMBackend; endpoint: string; api_key?: string };
export type LLMTestResult = { ok: boolean; message?: string; model?: string; error?: string };
export type LemonadeModelLifecycle = { state: 'unreachable' | 'model_absent' | 'loading' | 'ready' | 'lifecycle_unavailable' | 'load_failed' | 'timed_out'; message: string };
export type InsightTrigger = 'manual' | 'on_change' | 'scheduled';
export type InsightScheduleMode = 'daily' | 'repeat' | 'cron';
export type InsightDateScope = 'all' | 'today' | 'tomorrow' | 'this_week' | 'next_7_days' | 'custom';
export type InsightInquiry = { name: string; question: string; calendar_ids?: string[]; tags?: string[]; date_scope?: InsightDateScope; start_date?: string; end_date?: string; trigger: InsightTrigger; schedule_mode?: InsightScheduleMode; schedule?: string; repeat_interval?: string; cron_expression?: string; enabled: boolean; builtin?: boolean };
export type Insight = { name: string; question: string; answer?: string; evidence?: string[]; caveat?: string; source_hash?: string; source_at?: string; generated_at?: string; stale?: boolean; error?: string; calendar_ids?: string[]; tags?: string[]; schedule?: string };

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

/** Fetch a REST explorer response without assuming it is JSON. */
export async function apiText(path: string, init: RequestInit = {}): Promise<{ text: string; contentType: string }> {
  const headers = new Headers(init.headers);
  const token = sessionStorage.getItem(tokenKey);
  if (token) headers.set('Authorization', `Bearer ${token}`);
  const response = await fetch(path, { ...init, headers });
  if (!response.ok) throw new Error((await response.text()) || response.statusText);
  return { text: await response.text(), contentType: response.headers.get('content-type') || '' };
}

export const calendarAPI = {
  list: () => api<Calendar[]>('/api/calendars'),
  tags: () => api<Tag[]>('/api/tags'),
  add: (body: { name: string; url: string; tags: string[]; color?: string; icon?: string; refresh_interval?: string }) => api<Calendar>('/api/calendars', { method: 'POST', body: JSON.stringify(body) }),
  update: (id: string, body: Partial<Pick<Calendar, 'name' | 'url' | 'tags' | 'color' | 'icon' | 'refresh_interval' | 'enabled' | 'include_in_general_queries'>> & { tag_order?: string[]; clear_icon?: boolean; clear_refresh_interval?: boolean }) => api<Calendar>(`/api/calendars/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  updateTag: (name: string, body: Partial<Pick<Tag, 'color' | 'icon' | 'refresh_interval' | 'position'>> & { clear_icon?: boolean; clear_refresh_interval?: boolean }) => api<Tag>(`/api/tags/${encodeURIComponent(name)}`, { method: 'PUT', body: JSON.stringify(body) }),
  remove: (id: string) => api<void>(`/api/calendars/${id}`, { method: 'DELETE' }),
  refresh: (id: string) => api<void>(`/api/calendars/${id}/refresh`, { method: 'POST' }),
  refreshAll: () => api<unknown>('/api/rest/refresh_all_calendars', { method: 'POST', body: '{}' }),
  uploadCustomIcon: (id: string, file: File) => api<{ custom_icon_url: string }>(`/api/calendars/${id}/custom-icon`, { method: 'PUT', headers: { 'Content-Type': file.type }, body: file }),
  clearCustomIcon: (id: string) => api<void>(`/api/calendars/${id}/custom-icon`, { method: 'DELETE' }),
  validate: (url: string) => api<FeedValidation>('/api/calendars/validate', { method: 'POST', body: JSON.stringify({ url }) }),
  saveGeneralQuerySelection: (calendarIDs: string[]) => api<{ calendar_ids: string[] }>('/api/calendars/general-query-selection', { method: 'PUT', body: JSON.stringify({ calendar_ids: calendarIDs }) })
};

export const insightsAPI = {
  profile: () => api<LLMProfile>('/api/llm-profile'),
  saveProfile: (body: LLMProfileInput) => api<LLMProfile>('/api/llm-profile', { method: 'PUT', body: JSON.stringify(body) }),
  testProfile: () => api<LLMTestResult>('/api/llm-profile/test', { method: 'POST', body: '{}' }),
  testEndpoint: (body: LLMConnectionInput) => api<LLMTestResult>('/api/llm-profile/endpoint-test', { method: 'POST', body: JSON.stringify(body) }),
  discoverModels: (body: LLMConnectionInput) => api<{ models: string[] }>('/api/llm-profile/models', { method: 'POST', body: JSON.stringify(body) }),
  testModel: (body: LLMConnectionInput & { model: string }) => api<LLMTestResult>('/api/llm-profile/model-test', { method: 'POST', body: JSON.stringify(body) }),
  lemonadeModelStatus: (body: LLMConnectionInput & { model: string }) => api<LemonadeModelLifecycle>('/api/llm-profile/lemonade/model-status', { method: 'POST', body: JSON.stringify(body) }),
  loadLemonadeModel: (body: LLMConnectionInput & { model: string }) => api<LemonadeModelLifecycle>('/api/llm-profile/lemonade/model-load', { method: 'POST', body: JSON.stringify(body) }),
  list: () => api<Insight[]>('/api/insights'),
  run: (body: { name: string; question?: string; calendar_ids?: string[]; tags?: string[]; date_scope?: InsightDateScope; start_date?: string; end_date?: string }) => api<Insight>('/api/insights', { method: 'POST', body: JSON.stringify(body) }),
  preview: (body: { name?: string; question: string; calendar_ids?: string[]; tags?: string[]; date_scope?: InsightDateScope; start_date?: string; end_date?: string }) => api<Insight>('/api/insights/preview', { method: 'POST', body: JSON.stringify(body) }),
  inquiries: () => api<InsightInquiry[]>('/api/insight-inquiries'),
  saveInquiry: (inquiry: InsightInquiry, create = false) => api<InsightInquiry>(create ? '/api/insight-inquiries' : `/api/insight-inquiries/${encodeURIComponent(inquiry.name)}`, { method: create ? 'POST' : 'PUT', body: JSON.stringify(inquiry) }),
  deleteInquiry: (name: string) => api<void>(`/api/insight-inquiries/${encodeURIComponent(name)}`, { method: 'DELETE' }),
};
