import { describe, expect, it } from 'vitest';
import { eventPreviewDetails } from './EventPreview';

describe('event planner preview', () => {
  it('uses text-only event metadata and only shows optional fields when supplied', () => {
    const details = eventPreviewDetails({ name: '<b>Planning</b>', date: '2026-08-03', start: '10:00', end: '11:00', description: 'Location: Room 12', meeting_url: 'https://meet.example.test/x', attendance_status: 'tentative' }, { id: 'one', key: 'one', name: 'Work', url: '', tags: [], enabled: true, include_in_general_queries: true, event_count: 0 }, '12h');
    expect(details).toMatchObject({ calendarName: 'Work', time: '10:00 AM – 11:00 AM', rsvp: 'Tentative', location: 'Room 12', hasMeetingLink: true });
  });

  it('labels all-day events without inventing details', () => {
    expect(eventPreviewDetails({ name: 'Day off', all_day: true }, undefined, '24h')).toEqual({ calendarName: 'Calendar', time: 'All day', hasMeetingLink: false, rsvp: undefined, location: undefined });
  });
});
