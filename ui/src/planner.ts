import { Meeting } from './api';

export type TimedPlacement = {
  meeting: Meeting;
  top: number;
  height: number;
  lane: number;
  lanes: number;
};

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
