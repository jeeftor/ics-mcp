import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { EventCard } from './App';

describe('all-day event cards', () => {
  it('renders the event title instead of hiding it with the time label', () => {
    const title = 'Ruth Catlett PTO - Cruise';
    const markup = renderToStaticMarkup(<EventCard meeting={{ name: title, date: '2026-08-03', all_day: true }} selected={false} onSelect={() => undefined} timeFormat="12h" allDayPlacement={{ meeting: { name: title, date: '2026-08-03', all_day: true }, start: 0, span: 1, row: 0 }}/>);

    expect(markup).toContain(`class="event-title" style="display:inline">${title}</span>`);
  });

  it('renders only the normalized RSVP status, without attendee identity', () => {
    const markup = renderToStaticMarkup(<EventCard meeting={{ name: 'Planning', date: '2026-08-03', attendance_status: 'tentative' }} selected={false} onSelect={() => undefined} timeFormat="12h"/>);

    expect(markup).toContain('RSVP: Tentative');
    expect(markup).not.toContain('person@example.test');
  });

  it('adds an Outlook-style RSVP edge cue without replacing the calendar color', () => {
    const markup = renderToStaticMarkup(<EventCard meeting={{ name: 'Planning', date: '2026-08-03', attendance_status: 'tentative' }} selected={false} onSelect={() => undefined} timeFormat="12h"/>);

    expect(markup).toContain('event-card attendance-tentative');
    expect(markup).toContain('--event-color:#4f8f72;--attendance-color:#d49a24');
  });

  it('does not add an RSVP edge cue when the feed supplies no attendee status', () => {
    const markup = renderToStaticMarkup(<EventCard meeting={{ name: 'Planning', date: '2026-08-03' }} selected={false} onSelect={() => undefined} timeFormat="12h"/>);

    expect(markup).not.toContain('attendance-accepted');
    expect(markup).not.toContain('--attendance-color');
  });

  it('uses tall timed cards for three title lines and a useful secondary detail', () => {
    const markup = renderToStaticMarkup(<EventCard meeting={{ name: 'Cancelled: long planning meeting with the team', date: '2026-08-03', start: '12:00', end: '14:00', cancelled: true }} selected={false} onSelect={() => undefined} timeFormat="12h" placement={{ meeting: { name: 'Cancelled: long planning meeting with the team' }, top: 720, height: 120, lane: 0, lanes: 1 }}/>);

    expect(markup).toContain('three-line-title');
    expect(markup).toContain('has-event-secondary');
    expect(markup).toContain('event-card-secondary">Cancelled');
  });
});
