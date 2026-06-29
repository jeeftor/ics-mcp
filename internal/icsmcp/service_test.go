package icsmcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestCalendarEnvImportDerivesStableKeysAndPreservesRenamedDisplayName(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	existing, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "MITRE", Name: "Work Calendar", URL: "https://old.example/ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}

	env := map[string]string{
		"ICSMCP_CALENDAR_MITRE":        "https://new.example/mitre.ics",
		"ICSMCP_CALENDAR_EMILY_EVENTS": "https://example.test/emily.ics",
		"UNRELATED":                    "ignored",
	}
	if err := svc.ImportStartupCalendars(ctx, env, nil); err != nil {
		t.Fatalf("ImportStartupCalendars() error = %v", err)
	}

	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 2 {
		t.Fatalf("got %d calendars, want 2", len(calendars))
	}

	byKey := map[string]Calendar{}
	for _, cal := range calendars {
		byKey[cal.Key] = cal
	}
	if got := byKey["MITRE"].Name; got != "Work Calendar" {
		t.Fatalf("renamed display name = %q, want Work Calendar", got)
	}
	if got := byKey["MITRE"].URL; got != "https://new.example/mitre.ics" {
		t.Fatalf("updated URL = %q", got)
	}
	if byKey["MITRE"].ID != existing.ID {
		t.Fatalf("upsert changed stable ID: got %q want %q", byKey["MITRE"].ID, existing.ID)
	}
	if got := byKey["EMILY_EVENTS"].Name; got != "EMILY EVENTS" {
		t.Fatalf("derived display name = %q, want EMILY EVENTS", got)
	}
}

func TestParseICSExpandsRecurringEventsWithinWindow(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	events, err := ParseICS(sampleRecurringICS(), now, 5*24*time.Hour)
	if err != nil {
		t.Fatalf("ParseICS() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	wantStarts := []string{"2026-06-30T15:00:00Z", "2026-07-01T15:00:00Z", "2026-07-02T15:00:00Z"}
	for i, event := range events {
		if event.Name != "Daily Standup" || event.Start.Format(time.RFC3339) != wantStarts[i] {
			t.Fatalf("event[%d] = %#v, want daily standup at %s", i, event, wantStarts[i])
		}
	}
}

func TestParseICSExtractsOnlineMeetingURL(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	events, err := ParseICS(sampleTeamsICS(), now, 24*time.Hour)
	if err != nil {
		t.Fatalf("ParseICS() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].MeetingURL != "https://teams.microsoft.com/l/meetup-join/abc123" {
		t.Fatalf("meeting URL = %q", events[0].MeetingURL)
	}
	if events[0].MeetingURLType != "teams" {
		t.Fatalf("meeting URL type = %q", events[0].MeetingURLType)
	}
}

func TestUpcomingMeetingsIncludesOngoingAndDefaultsToTenSorted(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}

	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	var events []EventInstance
	for i := range 12 {
		start := now.Add(time.Duration(i+1) * time.Hour)
		events = append(events, EventInstance{
			CalendarID:   cal.ID,
			CalendarName: cal.Name,
			Name:         "Future",
			Start:        start,
			End:          start.Add(30 * time.Minute),
		})
	}
	events = append(events, EventInstance{
		CalendarID:     cal.ID,
		CalendarName:   cal.Name,
		Name:           "In Progress",
		Description:    "A long meeting description that should not be returned by default.",
		MeetingURL:     "https://teams.microsoft.com/l/meetup-join/abc123",
		MeetingURLType: "teams",
		Start:          now.Add(-15 * time.Minute),
		End:            now.Add(15 * time.Minute),
	})
	if err := svc.ReplaceEvents(ctx, cal.ID, events); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}

	got, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("got %d meetings, want default limit 10", len(got))
	}
	if got[0].Name != "In Progress" || !got[0].Ongoing {
		t.Fatalf("first meeting = %#v, want ongoing meeting first", got[0])
	}
	if got[0].Day != "Mon" {
		t.Fatalf("day label = %q, want Mon", got[0].Day)
	}
	if got[0].Description != "" {
		t.Fatalf("default description = %q, want empty", got[0].Description)
	}
	if got[0].MeetingURL != "https://teams.microsoft.com/l/meetup-join/abc123" || got[0].MeetingURLType != "teams" {
		t.Fatalf("meeting URL fields = %q %q", got[0].MeetingURL, got[0].MeetingURLType)
	}
	if !slices.IsSortedFunc(got, func(a, b Meeting) int {
		return a.StartTime.Compare(b.StartTime)
	}) {
		t.Fatalf("meetings are not sorted by start time: %#v", got)
	}

	withDescription, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 1, IncludeDescription: true, DescriptionMaxChars: 12})
	if err != nil {
		t.Fatalf("UpcomingMeetings(include description) error = %v", err)
	}
	if withDescription[0].Description != "A long meeti..." {
		t.Fatalf("opt-in description = %q, want truncated description", withDescription[0].Description)
	}
}

func TestUpcomingMeetingsByCalendarDefaultsToTenPerCalendar(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	work, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(work) error = %v", err)
	}
	home, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "home", Name: "Home", URL: "https://example.test/home.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(home) error = %v", err)
	}

	for _, cal := range []Calendar{work, home} {
		var events []EventInstance
		for i := range 12 {
			start := now.Add(time.Duration(i+1) * time.Hour)
			events = append(events, EventInstance{
				CalendarID:   cal.ID,
				CalendarName: cal.Name,
				Name:         cal.Name + " Future",
				Start:        start,
				End:          start.Add(30 * time.Minute),
			})
		}
		if err := svc.ReplaceEvents(ctx, cal.ID, events); err != nil {
			t.Fatalf("ReplaceEvents(%s) error = %v", cal.Name, err)
		}
	}

	chronological, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(chronological) != 10 {
		t.Fatalf("chronological meeting count = %d, want 10 total", len(chronological))
	}

	grouped, err := svc.UpcomingMeetingsByCalendar(ctx, UpcomingQuery{Now: now})
	if err != nil {
		t.Fatalf("UpcomingMeetingsByCalendar() error = %v", err)
	}
	if len(grouped) != 2 {
		t.Fatalf("group count = %d, want 2", len(grouped))
	}
	for _, group := range grouped {
		if len(group.Meetings) != 10 {
			t.Fatalf("group %s meeting count = %d, want 10 per calendar", group.CalendarName, len(group.Meetings))
		}
	}
}

func TestUpcomingMeetingsSupportsFilters(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	events := []EventInstance{
		{CalendarID: cal.ID, CalendarName: cal.Name, Name: "Current Planning", Description: "roadmap", Start: now.Add(-10 * time.Minute), End: now.Add(20 * time.Minute)},
		{CalendarID: cal.ID, CalendarName: cal.Name, Name: "Future Planning", Description: "roadmap", Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
		{CalendarID: cal.ID, CalendarName: cal.Name, Name: "All Day Planning", Start: now.Add(24 * time.Hour), End: now.Add(48 * time.Hour)},
		{CalendarID: cal.ID, CalendarName: cal.Name, Name: "Unrelated Sync", Start: now.Add(4 * time.Hour), End: now.Add(5 * time.Hour)},
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, events); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}

	got, err := svc.UpcomingMeetings(ctx, UpcomingQuery{
		Now:           now,
		Query:         "planning",
		OnlyOngoing:   true,
		ExcludeAllDay: true,
	})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "Current Planning" {
		t.Fatalf("filtered ongoing meetings = %#v", got)
	}

	windowed, err := svc.UpcomingMeetings(ctx, UpcomingQuery{
		Now:    now,
		After:  now.Add(90 * time.Minute),
		Before: now.Add(210 * time.Minute),
	})
	if err != nil {
		t.Fatalf("UpcomingMeetings(windowed) error = %v", err)
	}
	if len(windowed) != 1 || windowed[0].Name != "Future Planning" {
		t.Fatalf("windowed meetings = %#v", windowed)
	}
}

func TestValidateCalendarFetchesAndParsesFeedWithoutSaving(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	result, err := svc.ValidateCalendar(ctx, ValidateCalendarInput{URL: feed.URL, Limit: 2})
	if err != nil {
		t.Fatalf("ValidateCalendar() error = %v", err)
	}
	if !result.OK || result.EventCount != 1 || len(result.Meetings) != 1 || result.Meetings[0].Name != "Planning" {
		t.Fatalf("validation result = %#v", result)
	}
	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 0 {
		t.Fatalf("ValidateCalendar saved calendars = %#v", calendars)
	}
}

func TestRefreshPreservesLastKnownGoodEventsWhenFetchFails(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	fail := false
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "nope", http.StatusBadGateway)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	if err := svc.RefreshCalendar(ctx, cal.ID, now); err != nil {
		t.Fatalf("RefreshCalendar(first) error = %v", err)
	}
	fail = true
	if err := svc.RefreshCalendar(ctx, cal.ID, now); err == nil {
		t.Fatalf("RefreshCalendar(second) error = nil, want failure")
	}

	meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 5})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(meetings) != 1 || meetings[0].Name != "Planning" {
		t.Fatalf("cached meetings after failed refresh = %#v", meetings)
	}
	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Calendars[0].LastError == "" || status.Calendars[0].EventCount != 1 {
		t.Fatalf("status after failed refresh = %#v", status.Calendars[0])
	}
}
