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
		CalendarID:   cal.ID,
		CalendarName: cal.Name,
		Name:         "In Progress",
		Start:        now.Add(-15 * time.Minute),
		End:          now.Add(15 * time.Minute),
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
	if !slices.IsSortedFunc(got, func(a, b Meeting) int {
		return a.StartTime.Compare(b.StartTime)
	}) {
		t.Fatalf("meetings are not sorted by start time: %#v", got)
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
