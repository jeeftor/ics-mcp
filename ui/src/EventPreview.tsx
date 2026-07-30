import { ExternalLink, MapPin } from 'lucide-react';
import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type ReactElement, type RefObject } from 'react';
import { CalendarGlyph, calendarColor } from './CalendarGlyph';
import { meetingTime } from './calendarUtils';
import type { Calendar, Meeting } from './api';
import './event-preview.css';

type TriggerProps = {
  ref: RefObject<HTMLButtonElement | null>;
  onFocus: () => void;
  onBlur: () => void;
  onMouseEnter: () => void;
  onMouseLeave: () => void;
  onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => void;
  'aria-describedby'?: string;
};

type PreviewDetails = {
  calendarName: string;
  time: string;
  rsvp?: string;
  hasMeetingLink: boolean;
  location?: string;
};

/** Builds text-only metadata used by the planner preview; event content is never treated as HTML. */
export function eventPreviewDetails(meeting: Meeting, calendar: Calendar | undefined, timeFormat: '12h' | '24h'): PreviewDetails {
  const rsvp = meeting.attendance_status ? { accepted: 'Accepted', tentative: 'Tentative', declined: 'Declined', 'needs-action': 'No response' }[meeting.attendance_status] : undefined;
  const location = meeting.description?.match(/(?:^|\n)(?:location|where)\s*:\s*([^\n]+)/i)?.[1]?.trim();
  return {
    calendarName: calendar?.name || meeting.calendar_name || meeting.calendar || 'Calendar',
    time: meetingTime(meeting, timeFormat),
    rsvp,
    hasMeetingLink: Boolean(meeting.meeting_url),
    location: location || undefined,
  };
}

/** Returns the single useful, non-description detail permitted on a compact event card. */
export function eventCardSecondaryLine(meeting: Meeting): string | undefined {
  const location = meeting.description?.match(/(?:^|\n)(?:location|where)\s*:\s*([^\n]+)/i)?.[1]?.trim();
  return location || (meeting.meeting_url ? 'Online meeting' : undefined) || (meeting.cancelled ? 'Cancelled' : undefined);
}

/**
 * A fixed-position preview for compact event cards. It stays out of the grid,
 * flips above the card near the viewport edge, and can be dismissed with Escape.
 */
export function EventPreview({ meeting, calendar, timeFormat, children }: { meeting: Meeting; calendar?: Calendar; timeFormat: '12h' | '24h'; children: (props: TriggerProps) => ReactElement }) {
  const anchor = useRef<HTMLButtonElement>(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ left: 12, top: 12, above: false });
  const closeTimer = useRef<number | undefined>(undefined);
  const id = `event-preview-${[meeting.calendar_id, meeting.date, meeting.start, meeting.name].filter(Boolean).join('-').replace(/[^a-z0-9_-]+/gi, '-').slice(0, 80)}`;
  const details = eventPreviewDetails(meeting, calendar, timeFormat);
  const color = calendar ? calendarColor(calendar) : '#4f8f72';

  const cancelClose = () => { window.clearTimeout(closeTimer.current); };
  const scheduleClose = () => { closeTimer.current = window.setTimeout(() => setOpen(false), 100); };
  const updatePosition = () => {
    const rect = anchor.current?.getBoundingClientRect();
    if (!rect) return;
    const width = Math.min(380, window.innerWidth - 24);
    const left = Math.max(12, Math.min(rect.left + rect.width / 2 - width / 2, window.innerWidth - width - 12));
    const estimatedHeight = 180;
    const above = rect.bottom + 12 + estimatedHeight > window.innerHeight && rect.top > estimatedHeight;
    setPosition({ left, top: above ? Math.max(12, rect.top - 12) : rect.bottom + 12, above });
  };

  useLayoutEffect(() => { if (open) updatePosition(); }, [open]);
  useEffect(() => {
    if (!open) return;
    const reposition = () => updatePosition();
    window.addEventListener('resize', reposition);
    window.addEventListener('scroll', reposition, true);
    return () => { window.removeEventListener('resize', reposition); window.removeEventListener('scroll', reposition, true); };
  }, [open]);
  useEffect(() => () => window.clearTimeout(closeTimer.current), []);

  return <>{children({ ref: anchor, onFocus: () => { cancelClose(); setOpen(true); }, onBlur: scheduleClose, onMouseEnter: () => { cancelClose(); setOpen(true); }, onMouseLeave: scheduleClose, onKeyDown: event => { if (event.key === 'Escape') { event.preventDefault(); setOpen(false); anchor.current?.blur(); } }, 'aria-describedby': open ? id : undefined })}
    {open && <div id={id} className={`event-preview-popover${position.above ? ' above' : ''}`} role="tooltip" style={{ left: position.left, top: position.top, '--event-preview-color': color } as React.CSSProperties}>
      <div className="event-preview-calendar"><CalendarGlyph icon={calendar?.icon} color={color} size={18}/><span>{details.calendarName}</span></div>
      <strong>{meeting.name}</strong>
      <div className="event-preview-meta"><span>{details.time}</span>{details.rsvp && <span className={`event-preview-rsvp attendance-${meeting.attendance_status}`}>RSVP: {details.rsvp}</span>}{meeting.cancelled && <span className="event-preview-warning">Cancelled</span>}</div>
      {(details.location || details.hasMeetingLink) && <div className="event-preview-aux">{details.location && <span><MapPin size={14}/>{details.location}</span>}{details.hasMeetingLink && <span><ExternalLink size={14}/>Meeting link available</span>}</div>}
    </div>}</>;
}
