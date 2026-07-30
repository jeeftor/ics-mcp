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
});
