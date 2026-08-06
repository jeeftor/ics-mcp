import { Meeting } from './api';

/** Filters a fetched planner range using only the calendars visible in the UI. */
export function meetingsForVisibleCalendars(meetings: Meeting[], visibleIDs: string[]): Meeting[] {
  const visible = new Set(visibleIDs);
  return meetings.filter((meeting) => !meeting.calendar_id || visible.has(meeting.calendar_id));
}

export type AgendaGroupMode = 'date' | 'calendar' | 'all';
export type AgendaGroup = { key: string; label: string; meetings: Meeting[] };

/** Sorts agenda items predictably, keeping all-day events ahead of timed events on a given day. */
export function sortAgendaMeetings(meetings: Meeting[]): Meeting[] {
  return [...meetings].sort((left, right) => (
    (left.date || '').localeCompare(right.date || '')
    || Number(Boolean(right.all_day)) - Number(Boolean(left.all_day))
    || (left.start || '').localeCompare(right.start || '')
    || left.name.localeCompare(right.name)
  ));
}

/** Produces presentation groups for the agenda without changing the REST data contract. */
export function groupAgendaMeetings(meetings: Meeting[], mode: AgendaGroupMode, calendarName: (meeting: Meeting) => string): AgendaGroup[] {
  const sorted = sortAgendaMeetings(meetings);
  if (mode === 'all') return [{ key: 'all', label: '', meetings: sorted }];
  const groups = new Map<string, AgendaGroup>();
  for (const meeting of sorted) {
    const label = mode === 'date' ? meeting.date || 'Undated' : calendarName(meeting) || 'Calendar';
    const key = mode === 'date' ? `date:${label}` : `calendar:${label}`;
    const group = groups.get(key) || { key, label, meetings: [] };
    group.meetings.push(meeting);
    groups.set(key, group);
  }
  return [...groups.values()].sort((left, right) => mode === 'date' ? left.label.localeCompare(right.label) : left.label.localeCompare(right.label));
}

export type TimedPlacement = {
  meeting: Meeting;
  top: number;
  height: number;
  lane: number;
  lanes: number;
};

/**
 * Uses the card's actual vertical budget to decide whether a timed event can
 * safely show additional title lines. Short meetings stay single-line so their
 * labels do not collide with adjacent events, while long meetings can use the
 * otherwise empty vertical space.
 */
export function timedEventTitleLines(height: number, showTime: boolean): 1 | 2 | 3 | 4 | 5 {
  // Long cards should use their reserved vertical space before hiding the
  // subject. The cap still leaves room for the compact contextual detail.
  if (height >= (showTime ? 168 : 132)) return 5;
  if (height >= (showTime ? 138 : 106)) return 4;
  if (height >= (showTime ? 108 : 82)) return 3;
  return height >= (showTime ? 68 : 44) ? 2 : 1;
}

/** A clipped all-day event segment for the planner's current date range. */
export type AllDayPlacement = {
  meeting: Meeting;
  /** Zero-based visible day column. */
  start: number;
  /** Number of visible days occupied by this segment. */
  span: number;
  /** Zero-based collision-free all-day row. */
  row: number;
};

/**
 * Keeps the all-day strip compact until a person expands a specific day.
 * A multi-day event is revealed when any day it occupies is expanded, so its
 * bar stays continuous instead of being cut into misleading fragments.
 */
export function visibleAllDayPlacements(
  placements: AllDayPlacement[],
  days: number,
  maximumRows: number,
  expandedDays: ReadonlySet<number>,
): { visible: AllDayPlacement[]; overflowByDay: number[]; rows: number } {
  const overflowByDay = Array.from({ length: days }, () => 0);
  for (const placement of placements) {
    if (placement.row < maximumRows) continue;
    for (let day = placement.start; day < Math.min(days, placement.start + placement.span); day += 1) overflowByDay[day] += 1;
  }
  const visible = placements.filter(placement => placement.row < maximumRows || Array.from({ length: placement.span }, (_, index) => expandedDays.has(placement.start + index)).some(Boolean));
  const rows = Math.max(1, ...visible.map(placement => placement.row + 1));
  return { visible, overflowByDay, rows };
}

const minutesInDay = 24 * 60;

/** Returns the event's local start and end minutes, clamped to one calendar day. */
export function meetingMinutes(meeting: Meeting): { start: number; end: number } | undefined {
  if (!meeting.start) return undefined;
  const [hour, minute] = meeting.start.split(':').map(Number);
  if (!Number.isInteger(hour) || !Number.isInteger(minute)) return undefined;
  const start = Math.max(0, Math.min(minutesInDay - 1, hour * 60 + minute));
  const [endHour, endMinute] = (meeting.end || '').split(':').map(Number);
  const parsedEnd = Number.isInteger(endHour) && Number.isInteger(endMinute) ? endHour * 60 + endMinute : start + 30;
  return { start, end: Math.max(start + 15, Math.min(minutesInDay, parsedEnd)) };
}

/** Assigns horizontal lanes so overlapping timed events remain readable. */
export function placeTimedMeetings(meetings: Meeting[]): TimedPlacement[] {
  const sorted = meetings
    .map(meeting => ({ meeting, range: meetingMinutes(meeting) }))
    .filter((value): value is { meeting: Meeting; range: { start: number; end: number } } => value.range !== undefined)
    .sort((a, b) => a.range.start - b.range.start || a.range.end - b.range.end);
  const placed: TimedPlacement[] = [];
  let cluster: TimedPlacement[] = [];
  let clusterEnd = -1;
  const finishCluster = () => {
    const lanes = Math.max(1, ...cluster.map(item => item.lane + 1));
    cluster.forEach(item => { item.lanes = lanes; });
    cluster = [];
  };
  for (const { meeting, range } of sorted) {
    if (range.start >= clusterEnd && cluster.length) finishCluster();
    const occupied = new Set(cluster.filter(item => item.top + item.height > range.start).map(item => item.lane));
    let lane = 0;
    while (occupied.has(lane)) lane += 1;
    const item: TimedPlacement = { meeting, top: range.start, height: range.end - range.start, lane, lanes: 1 };
    cluster.push(item); placed.push(item); clusterEnd = Math.max(clusterEnd, range.end);
  }
  if (cluster.length) finishCluster();
  return placed;
}

/** Limits all-day rows while keeping overflow visible as a compact count. */
export function allDayRows(meetings: Meeting[], maximum = 3): { visible: Meeting[]; overflow: number } {
  return { visible: meetings.slice(0, maximum), overflow: Math.max(0, meetings.length - maximum) };
}

function addDateDays(date: string, days: number): string {
  const parsed = new Date(`${date}T12:00:00Z`);
  parsed.setUTCDate(parsed.getUTCDate() + days);
  return parsed.toISOString().slice(0, 10);
}

/**
 * Places all-day events as range-clipped spans.  ICS all-day DTEND is
 * exclusive; a missing or same-day end remains a one-day event for tolerant
 * handling of feeds that omit it.
 */
export function placeAllDayMeetings(meetings: Meeting[], rangeStart: string, days: number): AllDayPlacement[] {
  const rangeEnd = addDateDays(rangeStart, days);
  const candidates = meetings.flatMap(meeting => {
    const eventStart = meeting.date;
    if (!eventStart) return [];
    const suppliedEnd = meeting.end_date || addDateDays(eventStart, 1);
    const eventEnd = suppliedEnd <= eventStart ? addDateDays(eventStart, 1) : suppliedEnd;
    const start = eventStart > rangeStart ? eventStart : rangeStart;
    const end = eventEnd < rangeEnd ? eventEnd : rangeEnd;
    if (start >= end) return [];
    const offset = Math.round((Date.parse(`${start}T00:00:00Z`) - Date.parse(`${rangeStart}T00:00:00Z`)) / 86400000);
    const span = Math.round((Date.parse(`${end}T00:00:00Z`) - Date.parse(`${start}T00:00:00Z`)) / 86400000);
    return [{ meeting, start: offset, span }];
  }).sort((a, b) => a.start - b.start || b.span - a.span || a.meeting.name.localeCompare(b.meeting.name));

  const rowEnds: number[] = [];
  return candidates.map(candidate => {
    let row = rowEnds.findIndex(end => end <= candidate.start);
    if (row === -1) row = rowEnds.length;
    rowEnds[row] = candidate.start + candidate.span;
    return { ...candidate, row };
  });
}
