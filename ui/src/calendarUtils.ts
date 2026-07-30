import { Meeting } from './api';

export function startOfWeek(date: Date): Date {
  const result = new Date(date); const day = (result.getDay() + 6) % 7;
  result.setDate(result.getDate() - day); result.setHours(0, 0, 0, 0); return result;
}

export function addDays(date: Date, days: number): Date { const result = new Date(date); result.setDate(result.getDate() + days); return result; }
export function dateKey(date: Date): string { return date.toISOString().slice(0, 10); }
export function weekRangeLabel(start: Date): string {
  const end = addDays(start, 6); const options: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
  return `${start.toLocaleDateString(undefined, options)} – ${end.toLocaleDateString(undefined, { ...options, year: 'numeric' })}`;
}
export function meetingDate(meeting: Meeting): string { return meeting.date || ''; }
export function meetingTime(meeting: Meeting): string { return meeting.all_day ? 'All day' : `${meeting.start || ''}${meeting.end ? ` – ${meeting.end}` : ''}`; }
