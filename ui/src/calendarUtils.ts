import { Meeting } from './api';

export function startOfWeek(date: Date): Date {
  const result = new Date(date); const day = (result.getDay() + 6) % 7;
  result.setDate(result.getDate() - day); result.setHours(0, 0, 0, 0); return result;
}

/** Returns the current range anchor when the planner view is changed. */
export function plannerViewAnchor(view: 1 | 3 | 7, today = new Date()): Date {
  const result = new Date(today);
  result.setHours(0, 0, 0, 0);
  return view === 7 ? startOfWeek(result) : result;
}

export function addDays(date: Date, days: number): Date { const result = new Date(date); result.setDate(result.getDate() + days); return result; }
export function dateKey(date: Date): string { return date.toISOString().slice(0, 10); }
export function weekRangeLabel(start: Date): string {
  const end = addDays(start, 6); const options: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
  return `${start.toLocaleDateString(undefined, options)} – ${end.toLocaleDateString(undefined, { ...options, year: 'numeric' })}`;
}
export function meetingDate(meeting: Meeting): string { return meeting.date || ''; }
function formatClock(value: string | undefined, format: '12h' | '24h'): string {
  if (!value || format === '24h') return value || '';
  const [hours, minutes] = value.split(':').map(Number);
  if (Number.isNaN(hours) || Number.isNaN(minutes)) return value;
  const suffix = hours >= 12 ? 'PM' : 'AM'; const displayHours = hours % 12 || 12;
  return `${displayHours}:${String(minutes).padStart(2, '0')} ${suffix}`;
}

export function meetingTime(meeting: Meeting, format: '12h' | '24h' = '12h'): string {
  return meeting.all_day ? 'All day' : `${formatClock(meeting.start, format)}${meeting.end ? ` – ${formatClock(meeting.end, format)}` : ''}`;
}
