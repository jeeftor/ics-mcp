import * as Dialog from '@radix-ui/react-dialog';
import { Activity, CalendarDays, Check, Code2, Copy, ExternalLink, LoaderCircle, Plus, RefreshCw, Settings2, Tag, Terminal, X } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { api, Calendar, calendarAPI, Meeting, parseTags, RuntimeConfig, Status, Tag as CalendarTag, Tool } from './api';

type Page = 'dashboard' | 'calendars' | 'meetings' | 'config' | 'api';

const nav: Array<{ id: Page; label: string; icon: typeof CalendarDays }> = [
  { id: 'dashboard', label: 'Dashboard', icon: Activity },
  { id: 'calendars', label: 'Calendars', icon: CalendarDays },
  { id: 'meetings', label: 'Meetings', icon: CalendarDays },
  { id: 'config', label: 'Config', icon: Settings2 },
  { id: 'api', label: 'API', icon: Code2 }
];

function formatDate(value?: string) {
  return value ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : 'Never';
}

function relativeDate(value?: string) {
  if (!value) return 'Never refreshed';
  const minutes = Math.round((Date.now() - new Date(value).getTime()) / 60000);
  if (minutes < 1) return 'Updated just now';
  if (minutes < 60) return `Updated ${minutes}m ago`;
  return `Updated ${Math.round(minutes / 60)}h ago`;
}

export function App() {
  const [page, setPage] = useState<Page>('dashboard');
  const [status, setStatus] = useState<Status>();
  const [calendars, setCalendars] = useState<Calendar[]>([]);
  const [tags, setTags] = useState<CalendarTag[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [addOpen, setAddOpen] = useState(false);
  const [refreshing, setRefreshing] = useState(false);

  const reload = async () => {
    setLoading(true);
    try {
      const [nextStatus, nextCalendars, nextTags] = await Promise.all([api<Status>('/api/status'), calendarAPI.list(), calendarAPI.tags()]);
      setStatus(nextStatus); setCalendars(nextCalendars); setTags(nextTags); setError('');
    } catch (err) { setError(err instanceof Error ? err.message : 'Unable to load ICS MCP'); }
    finally { setLoading(false); }
  };
  useEffect(() => { void reload(); }, []);

  const refreshAll = async () => {
    setRefreshing(true);
    try { await calendarAPI.refreshAll(); await reload(); } finally { setRefreshing(false); }
  };

  const stale = calendars.filter((calendar) => calendar.last_error || !calendar.last_success).length;
  return <div className="app-shell">
    <header className="toolbar">
      <div className="brand"><div className="brand-mark">IC</div><div><strong>ICS MCP</strong><span>Calendar control center</span></div><code>v{status?.version.version || '…'}</code></div>
      <nav aria-label="Primary navigation">{nav.map((item) => { const Icon = item.icon; return <button key={item.id} className={page === item.id ? 'nav-link active' : 'nav-link'} onClick={() => setPage(item.id)}><Icon size={16}/>{item.label}</button>; })}</nav>
      <div className="toolbar-actions"><button className="icon-button" title="Server health" onClick={() => setPage('dashboard')}><span className={stale ? 'health-dot warning' : 'health-dot'} /></button><button className="button secondary" onClick={() => void refreshAll()} disabled={refreshing}>{refreshing ? <LoaderCircle className="spin" size={16}/> : <RefreshCw size={16}/>}Refresh all</button><button className="button primary" onClick={() => setAddOpen(true)}><Plus size={17}/>Add calendar</button></div>
    </header>
    <main>
      {error && <div className="notice error"><X size={16}/>{error}<button onClick={() => void reload()}>Try again</button></div>}
      {loading && !status ? <LoadingPage/> : <>
        {page === 'dashboard' && <Dashboard status={status} calendars={calendars} tags={tags} onCalendars={() => setPage('calendars')} />}
        {page === 'calendars' && <Calendars calendars={calendars} tags={tags} reload={reload} />}
        {page === 'meetings' && <Meetings calendars={calendars} />}
        {page === 'config' && <Config />}
        {page === 'api' && <APIWorkspace />}
      </>}
    </main>
    <AddCalendar open={addOpen} onOpenChange={setAddOpen} tags={tags} onAdded={reload}/>
  </div>;
}

function LoadingPage() { return <div className="loading-page"><LoaderCircle className="spin" size={28}/><p>Loading your calendars…</p></div>; }

function PageHeader({ eyebrow, title, description, action }: { eyebrow: string; title: string; description: string; action?: React.ReactNode }) { return <div className="page-header"><div><span className="eyebrow">{eyebrow}</span><h1>{title}</h1><p>{description}</p></div>{action}</div>; }

function Dashboard({ status, calendars, tags, onCalendars }: { status?: Status; calendars: Calendar[]; tags: CalendarTag[]; onCalendars: () => void }) {
  const totalEvents = calendars.reduce((sum, calendar) => sum + calendar.event_count, 0);
  const healthy = calendars.filter((calendar) => !calendar.last_error).length;
  return <><PageHeader eyebrow="OVERVIEW" title="Calendar control center" description="Monitor refresh health, tags, and the calendars used for your default meeting queries." action={<button className="button secondary" onClick={onCalendars}>Manage calendars</button>}/>
    <section className="metric-grid"><Metric label="Calendars" value={String(calendars.length)} detail={`${calendars.filter(c => c.include_in_general_queries).length} in default queries`} icon={<CalendarDays/>}/><Metric label="Cached events" value={String(totalEvents)} detail="Across all enabled feeds" icon={<Activity/>}/><Metric label="Refresh health" value={`${healthy}/${calendars.length}`} detail={healthy === calendars.length ? 'All feeds healthy' : 'Needs attention'} icon={<Check/>}/><Metric label="Timezone" value={status?.timezone || '—'} detail="Server display timezone" icon={<Settings2/>}/></section>
    <section className="dashboard-grid"><div className="panel"><div className="panel-heading"><div><h2>Calendar health</h2><p>Latest feed status at a glance.</p></div><button className="text-button" onClick={onCalendars}>View all</button></div><div className="health-list">{calendars.map(calendar => <div className="health-row" key={calendar.id}><span className={calendar.last_error ? 'status-dot danger' : 'status-dot'} /><div><strong>{calendar.name}</strong><small>{relativeDate(calendar.last_success)}</small></div><span>{calendar.event_count} events</span></div>)}</div></div>
      <div className="panel"><div className="panel-heading"><div><h2>Tags</h2><p>Organize and filter calendar groups.</p></div><Tag size={18}/></div><div className="tag-cloud">{tags.length ? tags.map(tag => <span className="tag-chip" key={tag.name}>{tag.name}<b>{tag.calendar_count}</b></span>) : <Empty text="Add tags to organize your calendars."/>}</div></div>
    </section></>;
}

function Metric({ label, value, detail, icon }: { label: string; value: string; detail: string; icon: React.ReactNode }) { return <article className="metric-card"><span className="metric-icon">{icon}</span><span className="metric-label">{label}</span><strong>{value}</strong><small>{detail}</small></article>; }

function Calendars({ calendars, tags, reload }: { calendars: Calendar[]; tags: CalendarTag[]; reload: () => Promise<void> }) {
  const [query, setQuery] = useState(''); const [tag, setTag] = useState('all'); const [busyID, setBusyID] = useState('');
  const filtered = useMemo(() => calendars.filter(calendar => (tag === 'all' || calendar.tags.includes(tag)) && calendar.name.toLowerCase().includes(query.toLowerCase())), [calendars, query, tag]);
  const refresh = async (id: string) => { setBusyID(id); try { await calendarAPI.refresh(id); await reload(); } finally { setBusyID(''); } };
  const toggleDefault = async (calendar: Calendar) => { await calendarAPI.update(calendar.id, { include_in_general_queries: !calendar.include_in_general_queries }); await reload(); };
  const remove = async (calendar: Calendar) => { if (window.confirm(`Remove ${calendar.name} and its cached events?`)) { await calendarAPI.remove(calendar.id); await reload(); } };
  return <><PageHeader eyebrow="CALENDARS" title="Your calendar inventory" description="Filter feeds, manage tags, and keep default meeting queries intentional."/>
    <section className="panel table-panel"><div className="table-toolbar"><label className="search"><span>Search</span><input value={query} onChange={event => setQuery(event.target.value)} placeholder="Find a calendar"/></label><div className="filter-chips"><button className={tag === 'all' ? 'filter-chip active' : 'filter-chip'} onClick={() => setTag('all')}>All <b>{calendars.length}</b></button>{tags.map(item => <button className={tag === item.name ? 'filter-chip active' : 'filter-chip'} key={item.name} onClick={() => setTag(item.name)}>{item.name}<b>{item.calendar_count}</b></button>)}</div></div>
      <div className="responsive-table"><table><thead><tr><th>Calendar</th><th>Tags</th><th>Refresh</th><th>Events</th><th>Default query</th><th aria-label="Actions"/></tr></thead><tbody>{filtered.map(calendar => <tr key={calendar.id}><td><div className="calendar-name"><span className={calendar.last_error ? 'status-dot danger' : 'status-dot'} /><div><strong>{calendar.name}</strong><small>{calendar.key}</small></div></div></td><td><div className="tag-list">{calendar.tags.length ? calendar.tags.map(value => <span className="tag-chip" key={value}>{value}</span>) : <span className="muted">No tags</span>}</div></td><td><div className="refresh-cell"><strong>{relativeDate(calendar.last_success)}</strong><small>{calendar.last_error || `Next ${formatDate(calendar.next_refresh)}`}</small></div></td><td>{calendar.event_count}</td><td><button className={calendar.include_in_general_queries ? 'toggle on' : 'toggle'} onClick={() => void toggleDefault(calendar)} aria-pressed={calendar.include_in_general_queries}><span/></button></td><td><div className="row-actions"><button className="icon-button" title="Copy calendar ID" onClick={() => void navigator.clipboard.writeText(calendar.id)}><Copy size={16}/></button><button className="icon-button" title="Refresh calendar" disabled={busyID === calendar.id} onClick={() => void refresh(calendar.id)}><RefreshCw className={busyID === calendar.id ? 'spin' : ''} size={16}/></button><button className="icon-button danger-button" title="Remove calendar" onClick={() => void remove(calendar)}><X size={16}/></button></div></td></tr>)}</tbody></table>{filtered.length === 0 && <Empty text="No calendars match this filter."/>}</div></section></>;
}

function AddCalendar({ open, onOpenChange, tags, onAdded }: { open: boolean; onOpenChange: (open: boolean) => void; tags: CalendarTag[]; onAdded: () => Promise<void> }) {
  const [name, setName] = useState(''); const [url, setURL] = useState(''); const [tagText, setTagText] = useState(''); const [saving, setSaving] = useState(false); const [error, setError] = useState('');
  const save = async (event: React.FormEvent) => { event.preventDefault(); setSaving(true); try { await calendarAPI.add({ name, url, tags: parseTags(tagText) }); await onAdded(); setName(''); setURL(''); setTagText(''); onOpenChange(false); } catch (err) { setError(err instanceof Error ? err.message : 'Unable to add calendar'); } finally { setSaving(false); } };
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal><Dialog.Overlay className="dialog-overlay"/><Dialog.Content className="dialog"><Dialog.Title>Add a calendar</Dialog.Title><Dialog.Description>Give the feed a clear name, ICS URL, and optional tags for filtering.</Dialog.Description><form onSubmit={save}><label>Name<input required value={name} onChange={event => setName(event.target.value)} placeholder="Work"/></label><label>ICS URL<input required type="url" value={url} onChange={event => setURL(event.target.value)} placeholder="https://…/calendar.ics"/></label><label>Tags<input value={tagText} onChange={event => setTagText(event.target.value)} placeholder="Work, Personal" list="tag-suggestions"/><datalist id="tag-suggestions">{tags.map(tag => <option key={tag.name} value={tag.name}/>)}</datalist></label>{error && <p className="form-error">{error}</p>}<div className="dialog-actions"><Dialog.Close asChild><button type="button" className="button secondary">Cancel</button></Dialog.Close><button className="button primary" disabled={saving}>{saving && <LoaderCircle className="spin" size={16}/>}Add calendar</button></div></form></Dialog.Content></Dialog.Portal></Dialog.Root>;
}

function Meetings({ calendars }: { calendars: Calendar[] }) {
  const [meetings, setMeetings] = useState<Meeting[]>([]); const [loading, setLoading] = useState(true);
  useEffect(() => { api<Meeting[]>('/api/meetings?window=today_tomorrow&sort=agenda').then(setMeetings).finally(() => setLoading(false)); }, []);
  return <><PageHeader eyebrow="MEETINGS" title="Your next meetings" description="A focused agenda from your calendars selected for default queries."/><section className="panel agenda">{loading ? <LoadingPage/> : meetings.length ? meetings.map(meeting => <article className="agenda-row" key={`${meeting.calendar_name}-${meeting.name}-${meeting.start_time}`}><span className={meeting.ongoing ? 'status-dot live' : 'status-dot'}/><div><small>{meeting.calendar_name || 'Calendar'}</small><strong>{meeting.name}</strong><span>{meeting.when || meeting.start_time}</span></div>{meeting.meeting_url && <a className="icon-button" href={meeting.meeting_url} target="_blank" rel="noreferrer"><ExternalLink size={16}/></a>}</article>) : <Empty text="No upcoming meetings in the selected calendars."/>}</section><p className="page-footnote">Need field selection, raw tool arguments, or custom windows? Use API → MCP Tools.</p></>;
}

function Config() {
  const [config, setConfig] = useState<RuntimeConfig>(); const [saving, setSaving] = useState('');
  const reload = () => api<RuntimeConfig>('/api/config').then(setConfig); useEffect(() => { void reload(); }, []);
  const save = async (key: keyof RuntimeConfig, value: string | boolean) => { setSaving(String(key)); try { await api<RuntimeConfig>('/api/config', { method: 'PUT', body: JSON.stringify({ [key]: value }) }).then(setConfig); } finally { setSaving(''); } };
  if (!config) return <LoadingPage/>;
  const fields: Array<[keyof RuntimeConfig, string, string, 'text' | 'checkbox']> = [['refresh_interval', 'Refresh interval', 'How often ICS feeds are refreshed.', 'text'], ['timezone', 'Display timezone', 'Used for meeting output and admin views.', 'text'], ['external_url', 'External URL', 'Shown in setup instructions and client snippets.', 'text'], ['update_check', 'Release checks', 'Periodically check for a newer ICS MCP release.', 'checkbox']];
  return <><PageHeader eyebrow="CONFIGURATION" title="Runtime settings" description="Changes persist in SQLite unless an environment variable or CLI flag overrides them."/><section className="settings-grid">{fields.map(([key, label, hint, kind]) => { const source = config.sources[key] || 'default'; const locked = source !== 'database' && source !== 'default'; const value = config[key]; return <article className="setting-card" key={String(key)}><div><h2>{label}</h2><p>{hint}</p><span className={locked ? 'source-lock' : 'source'}>{locked ? `Overridden by ${source}` : `Source: ${source}`}</span></div><div className="setting-control">{kind === 'checkbox' ? <input type="checkbox" checked={Boolean(value)} disabled={locked} onChange={event => void save(key, event.target.checked)}/> : <input defaultValue={String(value)} disabled={locked} onBlur={event => { if (event.target.value !== String(value)) void save(key, event.target.value); }}/>} {saving === key && <LoaderCircle className="spin" size={16}/>}</div></article>; })}</section></>;
}

function APIWorkspace() {
  const [mode, setMode] = useState<'mcp' | 'rest'>('mcp'); const [tools, setTools] = useState<Tool[]>([]); const [tool, setTool] = useState<Tool>(); const [args, setArgs] = useState('{}'); const [output, setOutput] = useState('{}');
  useEffect(() => { api<Tool[]>('/api/tools').then(items => { setTools(items); setTool(items[0]); setArgs(JSON.stringify(items[0]?.default_arguments || {}, null, 2)); }); }, []);
  const run = async () => { if (!tool) return; try { const result = await api<unknown>(mode === 'mcp' ? `/api/tools/${tool.name}/call` : `/api/rest/${tool.name}`, { method: 'POST', body: mode === 'mcp' ? JSON.stringify({ arguments: JSON.parse(args) }) : args }); setOutput(JSON.stringify(result, null, 2)); } catch (err) { setOutput(err instanceof Error ? err.message : 'Request failed'); } };
  return <><PageHeader eyebrow="API" title="Integration workspace" description="Inspect real MCP tools and REST responses without leaving the admin console."/><section className="panel api-workspace"><div className="subnav"><button className={mode === 'mcp' ? 'active' : ''} onClick={() => setMode('mcp')}><Terminal size={16}/>MCP Tools</button><button className={mode === 'rest' ? 'active' : ''} onClick={() => setMode('rest')}><Code2 size={16}/>REST</button></div><div className="api-grid"><aside>{tools.map(item => <button className={tool?.name === item.name ? 'tool-item active' : 'tool-item'} key={item.name} onClick={() => { setTool(item); setArgs(JSON.stringify(item.default_arguments || {}, null, 2)); }}><strong>{item.name}</strong><span>{item.description}</span></button>)}</aside><div><div className="api-heading"><div><h2>{tool?.name || 'Choose a tool'}</h2><p>{tool?.description}</p></div><button className="button primary" onClick={() => void run()}>Run request</button></div><label className="code-label">JSON arguments<textarea value={args} onChange={event => setArgs(event.target.value)} spellCheck="false"/></label><pre className="code-output">{output}</pre></div></div></section></>;
}

function Empty({ text }: { text: string }) { return <div className="empty"><CalendarDays size={22}/><p>{text}</p></div>; }
