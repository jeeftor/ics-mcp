package icsmcp

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
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

func TestImportStartupCalendarsReturnsEnvUpsertErrors(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	_, err := svc.store.db.ExecContext(ctx, `CREATE TRIGGER fail_startup_calendar_insert
		BEFORE INSERT ON calendars
		BEGIN
			SELECT RAISE(FAIL, 'blocked startup calendar insert');
		END`)
	if err != nil {
		t.Fatalf("CREATE TRIGGER error = %v", err)
	}

	err = svc.ImportStartupCalendars(ctx, map[string]string{
		"ICSMCP_CALENDAR_WORK": "https://example.test/work.ics",
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "insert calendar") {
		t.Fatalf("ImportStartupCalendars() error = %v, want insert calendar", err)
	}
}

func TestImportStartupCalendarsReturnsCLIUpsertErrors(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	_, err := svc.store.db.ExecContext(ctx, `CREATE TRIGGER fail_startup_cli_calendar_insert
		BEFORE INSERT ON calendars
		BEGIN
			SELECT RAISE(FAIL, 'blocked startup cli calendar insert');
		END`)
	if err != nil {
		t.Fatalf("CREATE TRIGGER error = %v", err)
	}

	err = svc.ImportStartupCalendars(ctx, nil, []string{"work=https://example.test/work.ics"})
	if err == nil || !strings.Contains(err.Error(), "insert calendar") {
		t.Fatalf("ImportStartupCalendars() error = %v, want insert calendar", err)
	}
}

func TestCLIStartupImportNormalizesKeysAndRejectsInvalidAssignments(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	if err := svc.ImportStartupCalendars(ctx, nil, []string{"Team Calendar=https://example.test/team.ics"}); err != nil {
		t.Fatalf("ImportStartupCalendars(valid CLI) error = %v", err)
	}
	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 1 {
		t.Fatalf("calendars = %#v, want 1", calendars)
	}
	if calendars[0].Key != "TEAM_CALENDAR" || calendars[0].Name != "Team Calendar" || calendars[0].URL != "https://example.test/team.ics" {
		t.Fatalf("CLI calendar = %#v", calendars[0])
	}

	for _, value := range []string{"missing-separator", "=https://example.test/no-key.ics", "NO_URL="} {
		if err := svc.ImportStartupCalendars(ctx, nil, []string{value}); err == nil || !strings.Contains(err.Error(), "calendar must be name=url") {
			t.Fatalf("ImportStartupCalendars(%q) error = %v, want assignment error", value, err)
		}
	}
}

func TestAddCalendarValidatesURLAndKeyOrName(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	for _, tc := range []struct {
		name string
		in   AddCalendarInput
		want string
	}{
		{name: "missing URL", in: AddCalendarInput{Key: "work", Name: "Work"}, want: "calendar URL is required"},
		{name: "missing key and name", in: AddCalendarInput{URL: "https://example.test/work.ics"}, want: "calendar key or name is required"},
		{name: "punctuation only key", in: AddCalendarInput{Key: "!!!", URL: "https://example.test/work.ics"}, want: "calendar key or name is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.AddCalendar(ctx, tc.in); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("AddCalendar() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestEnvMapIncludesCurrentProcessEnvironment(t *testing.T) {
	t.Setenv("ICSMCP_TEST_ENV_MAP", "present")
	env := EnvMap()
	if env["ICSMCP_TEST_ENV_MAP"] != "present" {
		t.Fatalf("EnvMap()[ICSMCP_TEST_ENV_MAP] = %q, want present", env["ICSMCP_TEST_ENV_MAP"])
	}
}

func TestOpenStoreReportsMigrationErrors(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/missing/icsmcp.sqlite3")
	if err == nil {
		_ = store.Close()
		t.Fatalf("OpenStore() error = nil, want missing parent directory error")
	}
	if !strings.Contains(err.Error(), "migrate sqlite") {
		t.Fatalf("OpenStore() error = %v, want migrate sqlite context", err)
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

func TestParseICSMapsExchangeWindowsTimezones(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	events, err := ParseICS(sampleExchangeWindowsTimezoneICS(), now, 48*time.Hour)
	if err != nil {
		t.Fatalf("ParseICS() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if got := events[0].Start.Format(time.RFC3339); got != "2026-06-30T13:00:00Z" {
		t.Fatalf("start = %s, want Eastern daylight converted to 13:00Z", got)
	}
	if got := events[0].End.Format(time.RFC3339); got != "2026-06-30T13:30:00Z" {
		t.Fatalf("end = %s, want Eastern daylight converted to 13:30Z", got)
	}
}

func TestParseICSHandlesMultipleTimezoneFormats(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		ics       string
		wantStart string
	}{
		{
			name:      "utc suffix",
			ics:       sampleTimezoneICS("20260630T150000Z", "20260630T153000Z"),
			wantStart: "2026-06-30T15:00:00Z",
		},
		{
			name:      "iana timezone",
			ics:       sampleTimezoneICS("TZID=America/Denver:20260630T090000", "TZID=America/Denver:20260630T093000"),
			wantStart: "2026-06-30T15:00:00Z",
		},
		{
			name:      "windows mountain timezone",
			ics:       sampleTimezoneICS("TZID=Mountain Standard Time:20260630T090000", "TZID=Mountain Standard Time:20260630T093000"),
			wantStart: "2026-06-30T15:00:00Z",
		},
		{
			name:      "windows eastern timezone",
			ics:       sampleTimezoneICS("TZID=Eastern Standard Time:20260630T090000", "TZID=Eastern Standard Time:20260630T093000"),
			wantStart: "2026-06-30T13:00:00Z",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := ParseICS(tt.ics, now, 48*time.Hour)
			if err != nil {
				t.Fatalf("ParseICS() error = %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}
			if got := events[0].Start.Format(time.RFC3339); got != tt.wantStart {
				t.Fatalf("start = %s, want %s", got, tt.wantStart)
			}
		})
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

func TestParseICSDetectsAllDayAndCancelledEvents(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	events, err := ParseICS(sampleCancelledAllDayICS(), now, 72*time.Hour)
	if err != nil {
		t.Fatalf("ParseICS() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if !events[0].AllDay {
		t.Fatalf("all day = false, want true")
	}
	if !events[0].Cancelled {
		t.Fatalf("cancelled = false, want true")
	}
}

func TestParseICSDefaultsUntitledEvents(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	events, err := ParseICS(sampleUntitledICS(), now, 24*time.Hour)
	if err != nil {
		t.Fatalf("ParseICS() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	if events[0].Name != "(untitled)" {
		t.Fatalf("name = %q, want (untitled)", events[0].Name)
	}
	if events[0].UID != "untitled-1" {
		t.Fatalf("UID = %q, want untitled-1", events[0].UID)
	}
}

func TestParseICSSkipsEventsMissingEnd(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	events, err := ParseICS("BEGIN:VCALENDAR\r\nVERSION:2.0\r\n"+
		"BEGIN:VEVENT\r\nUID:missing-end\r\nDTSTAMP:20260629T120000Z\r\nDTSTART:20260629T123000Z\r\nSUMMARY:Missing End\r\nEND:VEVENT\r\n"+
		"BEGIN:VEVENT\r\nUID:valid\r\nDTSTAMP:20260629T120000Z\r\nDTSTART:20260629T140000Z\r\nDTEND:20260629T143000Z\r\nSUMMARY:Valid Event\r\nEND:VEVENT\r\n"+
		"END:VCALENDAR\r\n", now, 24*time.Hour)
	if err != nil {
		t.Fatalf("ParseICS() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want only the valid event: %#v", len(events), events)
	}
	if events[0].UID != "valid" || events[0].Name != "Valid Event" {
		t.Fatalf("event = %#v, want valid event only", events[0])
	}
}

func TestParseICSReportsMissingUIDErrors(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	_, err := ParseICS(sampleMissingUIDICS(), now, 24*time.Hour)
	if err == nil || !strings.Contains(err.Error(), "could not parse event without UID") {
		t.Fatalf("ParseICS(missing UID) error = %v, want missing UID parse error", err)
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

func TestMeetingDescriptionHonorsOptInAndLengthBounds(t *testing.T) {
	description := "short description"
	if got := meetingDescription(description, UpcomingQuery{}); got != "" {
		t.Fatalf("meetingDescription(no opt-in) = %q, want empty", got)
	}
	if got := meetingDescription(description, UpcomingQuery{IncludeDescription: true, DescriptionMaxChars: len([]rune(description))}); got != description {
		t.Fatalf("meetingDescription(exact length) = %q, want original", got)
	}
	if got := meetingDescription(strings.Repeat("a", 301), UpcomingQuery{IncludeDescription: true}); got != strings.Repeat("a", 300)+"..." {
		t.Fatalf("meetingDescription(default max) length = %d, want 303", len([]rune(got)))
	}
	if got := meetingDescription("abcdef", UpcomingQuery{IncludeDescription: true, DescriptionMaxChars: 3}); got != "abc..." {
		t.Fatalf("meetingDescription(truncated) = %q, want abc...", got)
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

func TestUpcomingMeetingsHonorsGeneralQueryCalendarSelection(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	general, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "general", Name: "General", URL: "https://example.test/general.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(general) error = %v", err)
	}
	private, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "private", Name: "Private", URL: "https://example.test/private.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(private) error = %v", err)
	}
	if _, err := svc.UpdateCalendar(ctx, private.ID, UpdateCalendarInput{IncludeInGeneralQueries: ptr(false)}); err != nil {
		t.Fatalf("UpdateCalendar() error = %v", err)
	}
	for _, cal := range []Calendar{general, private} {
		if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
			CalendarID: cal.ID,
			Name:       cal.Name + " Meeting",
			Start:      now.Add(time.Hour),
			End:        now.Add(90 * time.Minute),
		}}); err != nil {
			t.Fatalf("ReplaceEvents(%s) error = %v", cal.Name, err)
		}
	}

	defaultMeetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 10})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if got := meetingNames(defaultMeetings); !slices.Equal(got, []string{"General Meeting"}) {
		t.Fatalf("default meeting names = %#v", got)
	}

	explicitPrivate, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 10, CalendarIDs: []string{private.ID}})
	if err != nil {
		t.Fatalf("UpcomingMeetings(explicit private) error = %v", err)
	}
	if got := meetingNames(explicitPrivate); !slices.Equal(got, []string{"Private Meeting"}) {
		t.Fatalf("explicit private meeting names = %#v", got)
	}

	defaultGroups, err := svc.UpcomingMeetingsByCalendar(ctx, UpcomingQuery{Now: now, Limit: 10})
	if err != nil {
		t.Fatalf("UpcomingMeetingsByCalendar() error = %v", err)
	}
	if len(defaultGroups) != 1 || defaultGroups[0].CalendarID != general.ID {
		t.Fatalf("default groups = %#v, want only general calendar", defaultGroups)
	}

	explicitGroups, err := svc.UpcomingMeetingsByCalendar(ctx, UpcomingQuery{Now: now, Limit: 10, CalendarIDs: []string{private.ID}})
	if err != nil {
		t.Fatalf("UpcomingMeetingsByCalendar(explicit private) error = %v", err)
	}
	if len(explicitGroups) != 1 || explicitGroups[0].CalendarID != private.ID {
		t.Fatalf("explicit groups = %#v, want private calendar", explicitGroups)
	}
}

func TestSetGeneralQueryCalendarsPersistsBulkSelection(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	work, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(work) error = %v", err)
	}
	home, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "home", Name: "Home", URL: "https://example.test/home.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(home) error = %v", err)
	}

	selection, err := svc.SetGeneralQueryCalendars(ctx, []string{home.ID})
	if err != nil {
		t.Fatalf("SetGeneralQueryCalendars() error = %v", err)
	}
	if !slices.Equal(selection.CalendarIDs, []string{home.ID}) {
		t.Fatalf("selection = %#v, want home only", selection)
	}

	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	byID := map[string]Calendar{}
	for _, cal := range calendars {
		byID[cal.ID] = cal
	}
	if byID[work.ID].IncludeInGeneralQueries || !byID[home.ID].IncludeInGeneralQueries {
		t.Fatalf("calendar inclusion after bulk save = %#v", byID)
	}

	selection, err = svc.SetGeneralQueryCalendars(ctx, []string{"", home.ID, home.ID, " "})
	if err != nil {
		t.Fatalf("SetGeneralQueryCalendars(duplicates) error = %v", err)
	}
	if !slices.Equal(selection.CalendarIDs, []string{home.ID}) {
		t.Fatalf("deduplicated selection = %#v, want home only", selection)
	}

	selection, err = svc.SetGeneralQueryCalendars(ctx, nil)
	if err != nil {
		t.Fatalf("SetGeneralQueryCalendars(nil) error = %v", err)
	}
	if len(selection.CalendarIDs) != 0 {
		t.Fatalf("empty selection = %#v, want no selected calendars", selection)
	}

	if _, err := svc.SetGeneralQueryCalendars(ctx, []string{"missing"}); err == nil || !strings.Contains(err.Error(), "unknown calendar id") {
		t.Fatalf("SetGeneralQueryCalendars(missing) error = %v, want unknown calendar id", err)
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
		{CalendarID: cal.ID, CalendarName: cal.Name, Name: "All Day Planning", AllDay: true, Start: now.Add(24 * time.Hour), End: now.Add(48 * time.Hour)},
		{CalendarID: cal.ID, CalendarName: cal.Name, Name: "Canceled: Planning", Cancelled: true, Start: now.Add(5 * time.Hour), End: now.Add(6 * time.Hour)},
		{CalendarID: cal.ID, CalendarName: cal.Name, Name: "Unrelated Sync", Start: now.Add(4 * time.Hour), End: now.Add(5 * time.Hour)},
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, events); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}

	got, err := svc.UpcomingMeetings(ctx, UpcomingQuery{
		Now:              now,
		Query:            "planning",
		OnlyOngoing:      true,
		ExcludeAllDay:    true,
		ExcludeCancelled: true,
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

func TestUpcomingMeetingsRendersConfiguredTimezone(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := NewService(store, ServiceOptions{RefreshInterval: 5 * time.Minute, Lookahead: 30 * 24 * time.Hour, Timezone: "America/Denver"})
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		CalendarID:   cal.ID,
		CalendarName: cal.Name,
		Name:         "Morning Planning",
		Start:        time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC),
		End:          time.Date(2026, 6, 29, 15, 30, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}

	meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 1})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(meetings) != 1 {
		t.Fatalf("meetings = %#v", meetings)
	}
	if meetings[0].Start != "09:00" || meetings[0].End != "09:30" || meetings[0].Timezone != "America/Denver" {
		t.Fatalf("timezone-rendered meeting = %#v", meetings[0])
	}

	utcMeetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 1, Timezone: "UTC"})
	if err != nil {
		t.Fatalf("UpcomingMeetings(UTC override) error = %v", err)
	}
	if utcMeetings[0].Start != "15:00" || utcMeetings[0].End != "15:30" || utcMeetings[0].Timezone != "UTC" {
		t.Fatalf("timezone-overridden meeting = %#v", utcMeetings[0])
	}
}

func TestUpcomingMeetingsByCalendarReportsInvalidTimezone(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		CalendarID: cal.ID,
		Name:       "Planning",
		Start:      now.Add(time.Hour),
		End:        now.Add(90 * time.Minute),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}

	groups, err := svc.UpcomingMeetingsByCalendar(ctx, UpcomingQuery{Now: now, Timezone: "America/Denbver"})
	if err == nil || !strings.Contains(err.Error(), "America/Denbver") {
		t.Fatalf("UpcomingMeetingsByCalendar() groups=%#v error=%v, want timezone error", groups, err)
	}
}

func TestServiceResolvesTimezoneFormats(t *testing.T) {
	tests := []struct {
		name         string
		timezone     string
		wantTimezone string
	}{
		{name: "iana", timezone: "America/Denver", wantTimezone: "America/Denver"},
		{name: "utc", timezone: "UTC", wantTimezone: "UTC"},
		{name: "windows", timezone: "Mountain Standard Time", wantTimezone: "America/Denver"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
			if err != nil {
				t.Fatalf("OpenStore() error = %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			svc := NewService(store, ServiceOptions{Timezone: tt.timezone})
			status, err := svc.Status(context.Background())
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
			if status.Timezone != tt.wantTimezone {
				t.Fatalf("timezone = %q, want %q", status.Timezone, tt.wantTimezone)
			}
		})
	}
}

func TestServiceDefaultTimezoneIgnoresContainerTZ(t *testing.T) {
	t.Setenv("TZ", "America/Denver")
	originalLocal := time.Local
	time.Local = time.FixedZone("Test/Local", 0)
	t.Cleanup(func() { time.Local = originalLocal })

	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := NewService(store, ServiceOptions{})

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Timezone != "UTC" {
		t.Fatalf("timezone = %q, want UTC", status.Timezone)
	}
}

func TestServiceWarnsAndDefaultsUTCForInvalidTimezone(t *testing.T) {
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	svc := NewService(store, ServiceOptions{Timezone: "America/Denbver", Logger: logger})

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Timezone != "UTC" {
		t.Fatalf("timezone = %q, want UTC", status.Timezone)
	}
	gotLogs := logs.String()
	for _, want := range []string{"timezone not recognized, defaulting to UTC", "America/Denbver"} {
		if !strings.Contains(gotLogs, want) {
			t.Fatalf("logs missing %q:\n%s", want, gotLogs)
		}
	}
}

func TestStatusIncludesNormalizedExternalURL(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := NewService(store, ServiceOptions{RefreshInterval: 5 * time.Minute, ExternalURL: "https://ics-mcp.vookie.net/"})

	status, err := svc.Status(ctx)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.ExternalURL != "https://ics-mcp.vookie.net" {
		t.Fatalf("external URL = %q, want trimmed vookie URL", status.ExternalURL)
	}
}

func TestServiceMethodsPropagateClosedStoreErrors(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	svc := NewService(store, ServiceOptions{})
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "import startup calendars",
			call: func() error {
				return svc.ImportStartupCalendars(ctx, map[string]string{"ICSMCP_CALENDAR_WORK": "https://example.test/work.ics"}, nil)
			},
		},
		{
			name: "add calendar",
			call: func() error {
				_, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
				return err
			},
		},
		{
			name: "list calendars",
			call: func() error {
				_, err := svc.ListCalendars(ctx)
				return err
			},
		},
		{
			name: "list calendar status",
			call: func() error {
				_, err := svc.ListCalendarStatus(ctx)
				return err
			},
		},
		{
			name: "update calendar",
			call: func() error {
				_, err := svc.UpdateCalendar(ctx, "missing", UpdateCalendarInput{Name: "Renamed"})
				return err
			},
		},
		{
			name: "remove calendar",
			call: func() error {
				return svc.RemoveCalendar(ctx, "missing")
			},
		},
		{
			name: "replace events",
			call: func() error {
				return svc.ReplaceEvents(ctx, "missing", []EventInstance{{UID: "event-1", Name: "Event", Start: time.Now(), End: time.Now().Add(time.Hour)}})
			},
		},
		{
			name: "refresh calendar",
			call: func() error {
				return svc.RefreshCalendar(ctx, "missing", time.Now())
			},
		},
		{
			name: "upcoming meetings",
			call: func() error {
				_, err := svc.UpcomingMeetings(ctx, UpcomingQuery{})
				return err
			},
		},
		{
			name: "upcoming meetings by calendar",
			call: func() error {
				_, err := svc.UpcomingMeetingsByCalendar(ctx, UpcomingQuery{})
				return err
			},
		},
		{
			name: "status",
			call: func() error {
				_, err := svc.Status(ctx)
				return err
			},
		},
		{
			name: "metrics",
			call: func() error {
				_, err := svc.MetricsText(ctx)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatalf("%s error = nil, want closed store error", tc.name)
			}
		})
	}
}

func TestStoreDeleteCalendarReportsTableSpecificErrors(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		table     string
		wantError string
	}{
		{name: "events table", table: "events", wantError: "delete events"},
		{name: "refresh state table", table: "refresh_state", wantError: "delete refresh state"},
		{name: "calendars table", table: "calendars", wantError: "delete calendar"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
			if err != nil {
				t.Fatalf("AddCalendar() error = %v", err)
			}
			if _, err := svc.store.db.ExecContext(ctx, "DROP TABLE "+tc.table); err != nil {
				t.Fatalf("DROP TABLE %s error = %v", tc.table, err)
			}

			err = svc.store.deleteCalendar(ctx, cal.ID)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("deleteCalendar() error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestStoreQueryEventsReportsCorruptCachedTimes(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		start     string
		end       string
		wantError string
	}{
		{name: "start", start: "2026-06-29T16:00:00", end: "2026-06-29T16:30:00Z", wantError: "parse event start"},
		{name: "end", start: "2026-06-29T16:00:00Z", end: "2026-06-29T16:30:00", wantError: "parse event end"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService(t)
			cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
			if err != nil {
				t.Fatalf("AddCalendar() error = %v", err)
			}
			_, err = svc.store.db.ExecContext(ctx, `INSERT INTO events
				(id, calendar_id, uid, name, description, start_time, end_time)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				"event-1", cal.ID, "uid-1", "Broken cache", "", tc.start, tc.end)
			if err != nil {
				t.Fatalf("insert corrupt event error = %v", err)
			}

			_, err = svc.store.queryEvents(ctx, time.Date(2026, 6, 29, 15, 0, 0, 0, time.UTC), time.Date(2026, 6, 29, 17, 0, 0, 0, time.UTC), nil, 10, false)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("queryEvents() error = %v, want %q", err, tc.wantError)
			}
		})
	}
}

func TestStoreQueryEventsReportsScanFailuresWithContext(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	start := time.Date(2026, 6, 29, 16, 0, 0, 0, time.UTC)
	_, err = svc.store.db.ExecContext(ctx, `INSERT INTO events
		(id, calendar_id, uid, name, description, cancelled, all_day, start_time, end_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"event-1", cal.ID, "uid-1", "Broken cache", "", "not-an-int", 0, start.Format(time.RFC3339Nano), start.Add(30*time.Minute).Format(time.RFC3339Nano))
	if err != nil {
		t.Fatalf("insert corrupt event error = %v", err)
	}

	_, err = svc.store.queryEvents(ctx, start.Add(-time.Hour), start.Add(time.Hour), nil, 10, false)
	if err == nil || !strings.Contains(err.Error(), "scan event") {
		t.Fatalf("queryEvents() error = %v, want scan event context", err)
	}
}

func TestStoreListCalendarsReportsScanFailuresWithContext(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if _, err := svc.store.db.ExecContext(ctx, `INSERT INTO calendars (id, key, name, url, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"calendar-1", "WORK", "Work", "https://example.test/work.ics", "not-an-int", "2026-06-29T12:00:00Z", "2026-06-29T12:00:00Z"); err != nil {
		t.Fatalf("insert corrupt calendar fixture error = %v", err)
	}

	_, err := svc.ListCalendars(ctx)
	if err == nil || !strings.Contains(err.Error(), "scan calendar") {
		t.Fatalf("ListCalendars() error = %v, want scan calendar context", err)
	}
}

func TestPlaceholdersFormatsSQLParameterLists(t *testing.T) {
	for _, tc := range []struct {
		count int
		want  string
	}{
		{count: 1, want: "?"},
		{count: 2, want: "?,?"},
		{count: 4, want: "?,?,?,?"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			if got := placeholders(tc.count); got != tc.want {
				t.Fatalf("placeholders(%d) = %q, want %q", tc.count, got, tc.want)
			}
		})
	}
}

func TestStoreUpsertCalendarReportsRefreshStateInsertFailure(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if _, err := svc.store.db.ExecContext(ctx, "DROP TABLE refresh_state"); err != nil {
		t.Fatalf("DROP TABLE refresh_state error = %v", err)
	}

	_, err := svc.store.upsertCalendar(ctx, Calendar{
		ID:                      "calendar-1",
		Key:                     "WORK",
		Name:                    "Work",
		URL:                     "https://example.test/work.ics",
		Enabled:                 true,
		IncludeInGeneralQueries: true,
	}, false)
	if err == nil || !strings.Contains(err.Error(), "insert refresh state") {
		t.Fatalf("upsertCalendar() error = %v, want insert refresh state", err)
	}
}

func TestStoreUpsertCalendarReportsCalendarInsertFailure(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	_, err := svc.store.db.ExecContext(ctx, `CREATE TRIGGER fail_calendar_insert
		BEFORE INSERT ON calendars
		BEGIN
			SELECT RAISE(FAIL, 'blocked calendar insert');
		END`)
	if err != nil {
		t.Fatalf("CREATE TRIGGER error = %v", err)
	}

	_, err = svc.store.upsertCalendar(ctx, Calendar{
		ID:                      "calendar-1",
		Key:                     "WORK",
		Name:                    "Work",
		URL:                     "https://example.test/work.ics",
		Enabled:                 true,
		IncludeInGeneralQueries: true,
	}, false)
	if err == nil || !strings.Contains(err.Error(), "insert calendar") {
		t.Fatalf("upsertCalendar() error = %v, want insert calendar", err)
	}
}

func TestStoreUpsertCalendarPreservesExistingEnabledState(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	original, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/old.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}

	updated, err := svc.store.upsertCalendar(ctx, Calendar{
		ID:                      original.ID,
		Key:                     "WORK",
		Name:                    "Work Renamed",
		URL:                     "https://example.test/new.ics",
		Enabled:                 false,
		IncludeInGeneralQueries: true,
	}, false)
	if err != nil {
		t.Fatalf("upsertCalendar() error = %v", err)
	}
	if !updated.Enabled {
		t.Fatalf("updated.Enabled = false, want existing enabled state preserved")
	}
	if updated.Name != "Work Renamed" || updated.URL != "https://example.test/new.ics" {
		t.Fatalf("updated calendar = %#v", updated)
	}
}

func TestStartupImportPreservesGeneralQuerySelection(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	original, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/old.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if _, err := svc.UpdateCalendar(ctx, original.ID, UpdateCalendarInput{IncludeInGeneralQueries: ptr(false)}); err != nil {
		t.Fatalf("UpdateCalendar() error = %v", err)
	}

	err = svc.ImportStartupCalendars(ctx, map[string]string{
		"ICSMCP_CALENDAR_WORK": "https://example.test/new.ics",
	}, nil)
	if err != nil {
		t.Fatalf("ImportStartupCalendars() error = %v", err)
	}

	persisted, err := svc.store.calendarByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("calendarByID() error = %v", err)
	}
	if persisted.IncludeInGeneralQueries {
		t.Fatalf("IncludeInGeneralQueries = true, want startup import to preserve false")
	}
	if persisted.URL != "https://example.test/new.ics" {
		t.Fatalf("URL = %q, want startup import to update feed URL", persisted.URL)
	}
}

func TestStoreUpsertCalendarReportsExistingUpdateFailure(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	original, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/old.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	_, err = svc.store.db.ExecContext(ctx, `CREATE TRIGGER fail_calendar_upsert_update
		BEFORE UPDATE ON calendars
		BEGIN
			SELECT RAISE(FAIL, 'blocked calendar upsert update');
		END`)
	if err != nil {
		t.Fatalf("CREATE TRIGGER error = %v", err)
	}

	_, err = svc.store.upsertCalendar(ctx, Calendar{
		ID:                      original.ID,
		Key:                     "WORK",
		Name:                    "Work Renamed",
		URL:                     "https://example.test/new.ics",
		Enabled:                 true,
		IncludeInGeneralQueries: true,
	}, false)
	if err == nil || !strings.Contains(err.Error(), "update calendar") {
		t.Fatalf("upsertCalendar() error = %v, want update calendar", err)
	}
}

func TestStoreUpdateCalendarReportsUpdateExecutionFailure(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	_, err = svc.store.db.ExecContext(ctx, `CREATE TRIGGER fail_calendar_update
		BEFORE UPDATE ON calendars
		BEGIN
			SELECT RAISE(FAIL, 'blocked calendar update');
		END`)
	if err != nil {
		t.Fatalf("CREATE TRIGGER error = %v", err)
	}

	_, err = svc.store.updateCalendar(ctx, cal.ID, UpdateCalendarInput{Name: "Renamed"})
	if err == nil || !strings.Contains(err.Error(), "update calendar") {
		t.Fatalf("updateCalendar() error = %v, want update calendar", err)
	}
}

func TestStoreUpdateCalendarCanUpdateOnlyURL(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	original, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/old.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}

	updated, err := svc.store.updateCalendar(ctx, original.ID, UpdateCalendarInput{URL: "https://example.test/new.ics"})
	if err != nil {
		t.Fatalf("updateCalendar() error = %v", err)
	}
	if updated.Name != "Work" || updated.URL != "https://example.test/new.ics" || !updated.Enabled {
		t.Fatalf("updated calendar = %#v", updated)
	}

	persisted, err := svc.store.calendarByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("calendarByID() error = %v", err)
	}
	if persisted.Name != "Work" || persisted.URL != "https://example.test/new.ics" || !persisted.Enabled {
		t.Fatalf("persisted calendar = %#v", persisted)
	}
}

func TestStoreReplaceEventsReportsInsertFailure(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	start := time.Date(2026, 6, 29, 16, 0, 0, 0, time.UTC)
	events := []EventInstance{
		{ID: "duplicate", UID: "uid-1", Name: "One", Start: start, End: start.Add(30 * time.Minute)},
		{ID: "duplicate", UID: "uid-2", Name: "Two", Start: start, End: start.Add(30 * time.Minute)},
	}

	err = svc.store.replaceEvents(ctx, cal.ID, events)
	if err == nil || !strings.Contains(err.Error(), "insert event") {
		t.Fatalf("replaceEvents() error = %v, want insert event", err)
	}
}

func TestStoreReplaceEventsReportsClearEventsFailure(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if _, err := svc.store.db.ExecContext(ctx, "DROP TABLE events"); err != nil {
		t.Fatalf("DROP TABLE events error = %v", err)
	}

	err = svc.store.replaceEvents(ctx, cal.ID, nil)
	if err == nil || !strings.Contains(err.Error(), "clear events") {
		t.Fatalf("replaceEvents() error = %v, want clear events", err)
	}
}

func TestStoreReplaceEventsRollsBackPartialInsertFailure(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	start := time.Date(2026, 6, 29, 16, 0, 0, 0, time.UTC)
	events := []EventInstance{
		{ID: "duplicate", UID: "uid-1", Name: "One", Start: start, End: start.Add(30 * time.Minute)},
		{ID: "duplicate", UID: "uid-2", Name: "Two", Start: start, End: start.Add(30 * time.Minute)},
	}

	if err := svc.store.replaceEvents(ctx, cal.ID, events); err == nil {
		t.Fatalf("replaceEvents() error = nil, want duplicate insert error")
	}
	cached, err := svc.store.queryEvents(ctx, start.Add(-time.Hour), start.Add(time.Hour), []string{cal.ID}, 10, true)
	if err != nil {
		t.Fatalf("queryEvents() error = %v", err)
	}
	if len(cached) != 0 {
		t.Fatalf("cached events after rolled back replace = %#v, want none", cached)
	}
}

func TestMetricsTextIncludesCalendarState(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()
	if _, err := svc.AddCalendarAndRefresh(ctx, AddCalendarInput{Key: "work", Name: `Work "Calendar"`, URL: feed.URL}); err != nil {
		t.Fatalf("AddCalendarAndRefresh() error = %v", err)
	}

	metrics, err := svc.MetricsText(ctx)
	if err != nil {
		t.Fatalf("MetricsText() error = %v", err)
	}
	for _, want := range []string{
		"# HELP icsmcp_calendars_total",
		"icsmcp_calendars_total 1",
		`calendar_key="WORK"`,
		`calendar_name="Work \"Calendar\""`,
		" 1\n",
	} {
		if !strings.Contains(metrics, want) {
			t.Fatalf("metrics missing %q:\n%s", want, metrics)
		}
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

func TestValidateCalendarUsesDefaultWindowAndLimitsPreviewMeetings(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleManyOneTimeICS(now, 12)))
	}))
	defer feed.Close()

	result, err := svc.ValidateCalendar(ctx, ValidateCalendarInput{URL: feed.URL})
	if err != nil {
		t.Fatalf("ValidateCalendar() error = %v", err)
	}
	if !result.OK || result.EventCount != 12 {
		t.Fatalf("validation result = %#v, want 12 parsed events", result)
	}
	if len(result.Meetings) != 10 {
		t.Fatalf("validation meetings = %d, want default limit 10", len(result.Meetings))
	}
	if result.Meetings[0].Name != "Preview 01" || result.Meetings[9].Name != "Preview 10" {
		t.Fatalf("validation meetings = %#v, want sorted first ten", result.Meetings)
	}
}

func TestValidateCalendarReportsParseFailuresWithoutSaving(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:bad\r\nDTSTART:not-a-date\r\nDTEND:20260629T130000Z\r\nSUMMARY:Bad\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"))
	}))
	defer feed.Close()

	result, err := svc.ValidateCalendar(ctx, ValidateCalendarInput{URL: feed.URL})
	if err == nil {
		t.Fatalf("ValidateCalendar() error = nil, want parse error")
	}
	if result.OK || result.StatusCode != http.StatusOK || result.Error == "" {
		t.Fatalf("validation result = %#v, want parse failure metadata", result)
	}
	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 0 {
		t.Fatalf("ValidateCalendar saved calendars = %#v", calendars)
	}
}

func TestValidateCalendarReportsRequestAndReadErrors(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if result, err := svc.ValidateCalendar(ctx, ValidateCalendarInput{URL: "http://[::1"}); err == nil || result.OK || result.Error == "" {
		t.Fatalf("ValidateCalendar(invalid URL) result=%#v error=%v, want request error", result, err)
	}

	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})}
	result, err := svc.ValidateCalendar(ctx, ValidateCalendarInput{URL: "https://example.test/feed.ics"})
	if err == nil || result.OK || !strings.Contains(result.Error, "dial failed") {
		t.Fatalf("ValidateCalendar(transport error) result=%#v error=%v, want transport error", result, err)
	}

	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       errReadCloser{err: errors.New("read failed")},
		}, nil
	})}
	result, err = svc.ValidateCalendar(ctx, ValidateCalendarInput{URL: "https://example.test/feed.ics"})
	if err == nil || result.OK || !strings.Contains(result.Error, "read failed") {
		t.Fatalf("ValidateCalendar(read error) result=%#v error=%v, want read error", result, err)
	}
}

func TestAddCalendarAndRefreshKeepsCalendarWhenRefreshFails(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary", http.StatusBadGateway)
	}))
	defer feed.Close()

	cal, err := svc.AddCalendarAndRefresh(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL})
	if err != nil {
		t.Fatalf("AddCalendarAndRefresh() error = %v", err)
	}
	if cal.Key != "WORK" || cal.Name != "Work" {
		t.Fatalf("saved calendar = %#v", cal)
	}
	statuses, err := svc.ListCalendarStatus(ctx)
	if err != nil {
		t.Fatalf("ListCalendarStatus() error = %v", err)
	}
	if len(statuses) != 1 || statuses[0].LastError == "" || statuses[0].LastAttempt == nil || statuses[0].EventCount != 0 {
		t.Fatalf("status after failed add refresh = %#v", statuses)
	}
}

func TestAddCalendarAndRefreshReturnsValidationErrors(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	_, err := svc.AddCalendarAndRefresh(ctx, AddCalendarInput{Key: "work", Name: "Work"})
	if err == nil || !strings.Contains(err.Error(), "calendar URL is required") {
		t.Fatalf("AddCalendarAndRefresh() error = %v, want missing URL", err)
	}
	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 0 {
		t.Fatalf("calendars after failed add-and-refresh = %#v, want none", calendars)
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

func TestRefreshPreservesLastKnownGoodEventsWhenParseFails(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	badFeed := false
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if badFeed {
			_, _ = w.Write([]byte("BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:bad\r\nDTSTART:not-a-date\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"))
			return
		}
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	if err := svc.RefreshCalendar(ctx, cal.ID, now); err != nil {
		t.Fatalf("RefreshCalendar(good feed) error = %v", err)
	}
	badFeed = true
	if err := svc.RefreshCalendar(ctx, cal.ID, now.Add(time.Minute)); err == nil {
		t.Fatalf("RefreshCalendar(parse failure) error = nil, want parse error")
	}
	meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 5})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(meetings) != 1 || meetings[0].Name != "Planning" {
		t.Fatalf("cached meetings after parse failure = %#v", meetings)
	}
	state, err := svc.store.refreshState(ctx, cal.ID)
	if err != nil {
		t.Fatalf("refreshState() error = %v", err)
	}
	if state.LastError == "" || state.EventCount != 1 {
		t.Fatalf("refreshState(parse failure) = %#v, want previous event count and parse error", state)
	}
}

func TestRefreshPreservesLastKnownGoodEventsWhenCacheReplaceFails(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	if err := svc.RefreshCalendar(ctx, cal.ID, now); err != nil {
		t.Fatalf("RefreshCalendar(good feed) error = %v", err)
	}
	if _, err := svc.store.db.ExecContext(ctx, `CREATE TRIGGER fail_refresh_event_insert
		BEFORE INSERT ON events
		BEGIN
			SELECT RAISE(FAIL, 'blocked event cache insert');
		END`); err != nil {
		t.Fatalf("CREATE TRIGGER error = %v", err)
	}

	if err := svc.RefreshCalendar(ctx, cal.ID, now.Add(time.Minute)); err == nil || !strings.Contains(err.Error(), "insert event") {
		t.Fatalf("RefreshCalendar(cache replace failure) error = %v, want insert event", err)
	}

	meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 5})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(meetings) != 1 || meetings[0].Name != "Planning" {
		t.Fatalf("cached meetings after cache replace failure = %#v", meetings)
	}
	state, err := svc.store.refreshState(ctx, cal.ID)
	if err != nil {
		t.Fatalf("refreshState() error = %v", err)
	}
	if state.LastError == "" || state.EventCount != 1 {
		t.Fatalf("refreshState(cache replace failure) = %#v, want previous event count and cache error", state)
	}
}

func TestRefreshCalendarRecordsRequestAndReadErrors(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	badURL, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "bad-url", Name: "Bad URL", URL: "http://[::1"})
	if err != nil {
		t.Fatalf("AddCalendar(bad URL) error = %v", err)
	}
	if err := svc.RefreshCalendar(ctx, badURL.ID, now); err == nil {
		t.Fatalf("RefreshCalendar(invalid URL) error = nil, want request error")
	}
	state, err := svc.store.refreshState(ctx, badURL.ID)
	if err != nil {
		t.Fatalf("refreshState(bad URL) error = %v", err)
	}
	if state.LastAttempt == nil || state.LastError == "" {
		t.Fatalf("refreshState(bad URL) = %#v, want recorded request error", state)
	}

	readError, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "read-error", Name: "Read Error", URL: "https://example.test/read.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(read error) error = %v", err)
	}
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       errReadCloser{err: errors.New("read failed")},
		}, nil
	})}
	if err := svc.RefreshCalendar(ctx, readError.ID, now); err == nil {
		t.Fatalf("RefreshCalendar(read error) error = nil, want read error")
	}
	state, err = svc.store.refreshState(ctx, readError.ID)
	if err != nil {
		t.Fatalf("refreshState(read error) error = %v", err)
	}
	if state.LastAttempt == nil || !strings.Contains(state.LastError, "read failed") {
		t.Fatalf("refreshState(read error) = %#v, want recorded read error", state)
	}
}

func TestRefreshCalendarReturnsRefreshStateLookupErrors(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if _, err := svc.store.db.ExecContext(ctx, `DROP TABLE refresh_state`); err != nil {
		t.Fatalf("DROP TABLE refresh_state error = %v", err)
	}

	err = svc.RefreshCalendar(ctx, cal.ID, time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC))
	if err == nil || !strings.Contains(err.Error(), "refresh_state") {
		t.Fatalf("RefreshCalendar() error = %v, want refresh_state lookup error", err)
	}
}

func TestRemoveCalendarIsIdempotentForMissingID(t *testing.T) {
	svc := newTestService(t)
	if err := svc.RemoveCalendar(context.Background(), "missing"); err != nil {
		t.Fatalf("RemoveCalendar(missing) error = %v", err)
	}
}

func TestRemoveCalendarDeletesCachedEventsAndRefreshState(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.RefreshCalendar(ctx, cal.ID, now); err != nil {
		t.Fatalf("RefreshCalendar() error = %v", err)
	}
	if err := svc.RemoveCalendar(ctx, cal.ID); err != nil {
		t.Fatalf("RemoveCalendar() error = %v", err)
	}

	meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 10})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(meetings) != 0 {
		t.Fatalf("meetings after remove = %#v, want none", meetings)
	}
	statuses, err := svc.ListCalendarStatus(ctx)
	if err != nil {
		t.Fatalf("ListCalendarStatus() error = %v", err)
	}
	if len(statuses) != 0 {
		t.Fatalf("statuses after remove = %#v, want none", statuses)
	}
	state, err := svc.store.refreshState(ctx, cal.ID)
	if err != nil {
		t.Fatalf("refreshState(removed) error = %v", err)
	}
	if state.LastAttempt != nil || state.LastSuccess != nil || state.LastError != "" || state.ETag != "" || state.EventCount != 0 {
		t.Fatalf("refreshState(removed) = %#v, want empty state", state)
	}
}

func TestListCalendarStatusHandlesMissingRefreshState(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if _, err := svc.store.db.ExecContext(ctx, `DELETE FROM refresh_state WHERE calendar_id = ?`, cal.ID); err != nil {
		t.Fatalf("delete refresh_state fixture: %v", err)
	}

	statuses, err := svc.ListCalendarStatus(ctx)
	if err != nil {
		t.Fatalf("ListCalendarStatus() error = %v", err)
	}
	if len(statuses) != 1 {
		t.Fatalf("ListCalendarStatus() = %#v, want one calendar", statuses)
	}
	status := statuses[0]
	if status.ID != cal.ID || status.Key != "WORK" || status.Name != "Work" || !status.Enabled {
		t.Fatalf("calendar status identity = %#v", status)
	}
	if status.LastAttempt != nil || status.LastSuccess != nil || status.NextRefresh != nil ||
		status.LastError != "" || status.ETag != "" || status.LastModified != "" || status.EventCount != 0 {
		t.Fatalf("refresh status = %#v, want zero-value refresh fields", status)
	}
}

func TestListCalendarStatusReportsScanFailuresWithContext(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if _, err := svc.store.db.ExecContext(ctx, `INSERT INTO calendars (id, key, name, url, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"calendar-1", "WORK", "Work", "https://example.test/work.ics", "not-an-int", "2026-06-29T12:00:00Z", "2026-06-29T12:00:00Z"); err != nil {
		t.Fatalf("insert corrupt calendar fixture error = %v", err)
	}

	_, err := svc.ListCalendarStatus(ctx)
	if err == nil || !strings.Contains(err.Error(), "scan calendar status") {
		t.Fatalf("ListCalendarStatus() error = %v, want scan calendar status context", err)
	}
}

func TestRefreshAllCalendarsReportsSuccessFailureAndSkipped(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	goodFeed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer goodFeed.Close()
	badFeed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "temporary", http.StatusBadGateway)
	}))
	defer badFeed.Close()

	good, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "good", Name: "Good", URL: goodFeed.URL})
	if err != nil {
		t.Fatalf("AddCalendar(good) error = %v", err)
	}
	bad, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "bad", Name: "Bad", URL: badFeed.URL})
	if err != nil {
		t.Fatalf("AddCalendar(bad) error = %v", err)
	}
	disabled, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "disabled", Name: "Disabled", URL: goodFeed.URL})
	if err != nil {
		t.Fatalf("AddCalendar(disabled) error = %v", err)
	}
	if _, err := svc.UpdateCalendar(ctx, disabled.ID, UpdateCalendarInput{Enabled: ptr(false)}); err != nil {
		t.Fatalf("UpdateCalendar(disabled) error = %v", err)
	}

	results, err := svc.RefreshAllCalendars(ctx)
	if err != nil {
		t.Fatalf("RefreshAllCalendars() error = %v", err)
	}
	byID := map[string]RefreshCalendarResult{}
	for _, result := range results {
		byID[result.CalendarID] = result
	}
	if len(byID) != 3 {
		t.Fatalf("refresh results = %#v, want 3", results)
	}
	if !byID[good.ID].OK || byID[good.ID].Skipped || byID[good.ID].Error != "" {
		t.Fatalf("good result = %#v", byID[good.ID])
	}
	if byID[bad.ID].OK || byID[bad.ID].Error == "" || byID[bad.ID].Skipped {
		t.Fatalf("bad result = %#v", byID[bad.ID])
	}
	if !byID[disabled.ID].OK || !byID[disabled.ID].Skipped || byID[disabled.ID].Error != "" {
		t.Fatalf("disabled result = %#v", byID[disabled.ID])
	}
}

func TestRefreshAllCalendarsReportsStatusListingFailures(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	if err := svc.store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	results, err := svc.RefreshAllCalendars(ctx)
	if err == nil || !strings.Contains(err.Error(), "list calendar status") {
		t.Fatalf("RefreshAllCalendars() error = %v, want status listing error", err)
	}
	if results != nil {
		t.Fatalf("RefreshAllCalendars() results = %#v, want nil on status listing failure", results)
	}
}

func TestReplaceEventsRollsBackOnDuplicateIDs(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	events := []EventInstance{
		{ID: "duplicate", UID: "first", Name: "First", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
		{ID: "duplicate", UID: "second", Name: "Second", Start: now.Add(3 * time.Hour), End: now.Add(4 * time.Hour)},
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, events); err == nil || !strings.Contains(err.Error(), "insert event") {
		t.Fatalf("ReplaceEvents() error = %v, want duplicate insert error", err)
	}
	meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 10})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(meetings) != 0 {
		t.Fatalf("meetings after rolled-back replace = %#v, want none", meetings)
	}
}

func TestRefreshStatePersistsAndParsesTimestamps(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 123, time.UTC)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	want := refreshState{
		LastAttempt:  ptr(now),
		LastSuccess:  ptr(now.Add(time.Minute)),
		LastError:    "temporary",
		NextRefresh:  ptr(now.Add(5 * time.Minute)),
		ETag:         `"v2"`,
		LastModified: "Mon, 29 Jun 2026 12:00:00 GMT",
		EventCount:   42,
	}
	if err := svc.store.updateRefreshState(ctx, cal.ID, want); err != nil {
		t.Fatalf("updateRefreshState() error = %v", err)
	}
	got, err := svc.store.refreshState(ctx, cal.ID)
	if err != nil {
		t.Fatalf("refreshState() error = %v", err)
	}
	if got.LastAttempt == nil || !got.LastAttempt.Equal(*want.LastAttempt) ||
		got.LastSuccess == nil || !got.LastSuccess.Equal(*want.LastSuccess) ||
		got.NextRefresh == nil || !got.NextRefresh.Equal(*want.NextRefresh) ||
		got.LastError != want.LastError || got.ETag != want.ETag || got.LastModified != want.LastModified || got.EventCount != want.EventCount {
		t.Fatalf("refreshState() = %#v, want %#v", got, want)
	}

	if parsed := parseTimePtr(sql.NullString{String: "not-a-time", Valid: true}); parsed != nil {
		t.Fatalf("parseTimePtr(invalid) = %v, want nil", parsed)
	}
}

func TestRefreshStateReportsScanFailuresWithContext(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if _, err := svc.store.db.ExecContext(ctx, `UPDATE refresh_state SET event_count = ? WHERE calendar_id = ?`, "not-an-int", cal.ID); err != nil {
		t.Fatalf("corrupt refresh_state fixture error = %v", err)
	}

	_, err = svc.store.refreshState(ctx, cal.ID)
	if err == nil || !strings.Contains(err.Error(), "scan refresh state") {
		t.Fatalf("refreshState() error = %v, want scan refresh state context", err)
	}
}

func TestDisabledCalendarsDoNotReturnCachedMeetings(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		ID:    "work-1",
		UID:   "work-uid",
		Name:  "Hidden Meeting",
		Start: now.Add(time.Hour),
		End:   now.Add(2 * time.Hour),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}
	if _, err := svc.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{Enabled: ptr(false)}); err != nil {
		t.Fatalf("UpdateCalendar(disable) error = %v", err)
	}

	meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 10})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(meetings) != 0 {
		t.Fatalf("disabled calendar meetings = %#v, want none", meetings)
	}
}

func TestRefreshCalendarSendsConditionalHeadersAndHandlesNotModified(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	requests := 0
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 2 {
			if got := r.Header.Get("If-None-Match"); got != `"v1"` {
				t.Fatalf("If-None-Match = %q, want ETag", got)
			}
			if got := r.Header.Get("If-Modified-Since"); got != "Mon, 29 Jun 2026 12:00:00 GMT" {
				t.Fatalf("If-Modified-Since = %q, want Last-Modified", got)
			}
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Mon, 29 Jun 2026 12:00:00 GMT")
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.RefreshCalendar(ctx, cal.ID, now); err != nil {
		t.Fatalf("RefreshCalendar(first) error = %v", err)
	}
	if err := svc.RefreshCalendar(ctx, cal.ID, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("RefreshCalendar(not modified) error = %v", err)
	}

	statuses, err := svc.ListCalendarStatus(ctx)
	if err != nil {
		t.Fatalf("ListCalendarStatus() error = %v", err)
	}
	if requests != 2 || statuses[0].LastError != "" || statuses[0].EventCount != 1 {
		t.Fatalf("requests=%d status=%#v", requests, statuses[0])
	}
	if statuses[0].LastSuccess == nil || !statuses[0].LastSuccess.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("last success = %v, want second attempt", statuses[0].LastSuccess)
	}
}

func TestRefreshDueCalendarsRefreshesDueAndSkipsFutureOrDisabled(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	requestsByPath := map[string]int{}
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsByPath[r.URL.Path]++
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	due, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "due", Name: "Due", URL: feed.URL + "/due.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(due) error = %v", err)
	}
	future, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "future", Name: "Future", URL: feed.URL + "/future.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(future) error = %v", err)
	}
	disabled, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "disabled", Name: "Disabled", URL: feed.URL + "/disabled.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(disabled) error = %v", err)
	}
	if _, err := svc.UpdateCalendar(ctx, disabled.ID, UpdateCalendarInput{Enabled: ptr(false)}); err != nil {
		t.Fatalf("UpdateCalendar(disabled) error = %v", err)
	}
	if err := svc.RefreshCalendar(ctx, future.ID, now); err != nil {
		t.Fatalf("RefreshCalendar(future) error = %v", err)
	}
	requestsByPath = map[string]int{}

	svc.RefreshDueCalendars(ctx)

	if requestsByPath["/due.ics"] != 1 {
		t.Fatalf("due refresh count = %d, want 1", requestsByPath["/due.ics"])
	}
	if requestsByPath["/future.ics"] != 0 {
		t.Fatalf("future refresh count = %d, want 0", requestsByPath["/future.ics"])
	}
	if requestsByPath["/disabled.ics"] != 0 {
		t.Fatalf("disabled refresh count = %d, want 0", requestsByPath["/disabled.ics"])
	}
	statuses, err := svc.ListCalendarStatus(ctx)
	if err != nil {
		t.Fatalf("ListCalendarStatus() error = %v", err)
	}
	statusByID := map[string]CalendarStatus{}
	for _, status := range statuses {
		statusByID[status.ID] = status
	}
	if statusByID[due.ID].EventCount != 1 || statusByID[due.ID].LastSuccess == nil {
		t.Fatalf("due status = %#v", statusByID[due.ID])
	}
	if statusByID[disabled.ID].LastAttempt != nil {
		t.Fatalf("disabled status = %#v, want no refresh attempt", statusByID[disabled.ID])
	}
}

func TestRefreshDueCalendarsLogsStatusScanFailures(t *testing.T) {
	ctx := context.Background()
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	var logs bytes.Buffer
	svc := NewService(store, ServiceOptions{Logger: slog.New(slog.NewTextHandler(&logs, nil))})
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	svc.RefreshDueCalendars(ctx)

	gotLogs := logs.String()
	if !strings.Contains(gotLogs, "calendar refresh scan failed") || !strings.Contains(gotLogs, "sql: database is closed") {
		t.Fatalf("logs = %q, want status scan failure", gotLogs)
	}
}

func TestRunRefresherRefreshesUntilContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := NewService(store, ServiceOptions{RefreshInterval: 10 * time.Millisecond, Lookahead: 30 * 24 * time.Hour})
	var requests atomic.Int32
	seenSecondRequest := make(chan struct{})
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 2 {
			close(seenSecondRequest)
		}
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()
	if _, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL}); err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.RunRefresher(ctx)
	}()

	select {
	case <-seenSecondRequest:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("RunRefresher made %d requests, want at least 2", requests.Load())
	}
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("RunRefresher did not stop after context cancellation")
	}
}

func TestUpcomingMeetingsFiltersByCalendarIDs(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	work, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(work) error = %v", err)
	}
	personal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "personal", Name: "Personal", URL: "https://example.test/personal.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(personal) error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, work.ID, []EventInstance{{
		ID:           "work-1",
		UID:          "work-uid",
		Name:         "Work Planning",
		Start:        now.Add(time.Hour),
		End:          now.Add(2 * time.Hour),
		CalendarName: "ignored",
	}}); err != nil {
		t.Fatalf("ReplaceEvents(work) error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, personal.ID, []EventInstance{{
		ID:    "personal-1",
		UID:   "personal-uid",
		Name:  "Personal Errand",
		Start: now.Add(30 * time.Minute),
		End:   now.Add(90 * time.Minute),
	}}); err != nil {
		t.Fatalf("ReplaceEvents(personal) error = %v", err)
	}

	meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Now: now, Limit: 10, CalendarIDs: []string{work.ID}})
	if err != nil {
		t.Fatalf("UpcomingMeetings() error = %v", err)
	}
	if len(meetings) != 1 || meetings[0].Name != "Work Planning" || meetings[0].CalendarName != "Work" {
		t.Fatalf("filtered meetings = %#v", meetings)
	}
}

func ptr[T any](value T) *T {
	return &value
}

func sampleManyOneTimeICS(start time.Time, count int) string {
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\n")
	for i := 0; i < count; i++ {
		eventStart := start.Add(time.Duration(i+1) * time.Hour).UTC()
		eventEnd := eventStart.Add(30 * time.Minute)
		_, _ = fmt.Fprintf(&b, "BEGIN:VEVENT\r\nUID:preview-%02d\r\nDTSTAMP:%s\r\nDTSTART:%s\r\nDTEND:%s\r\nSUMMARY:Preview %02d\r\nEND:VEVENT\r\n",
			i+1, start.UTC().Format("20060102T150405Z"), eventStart.Format("20060102T150405Z"), eventEnd.Format("20060102T150405Z"), i+1)
	}
	b.WriteString("END:VCALENDAR\r\n")
	return b.String()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errReadCloser struct {
	err error
}

func (e errReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e errReadCloser) Close() error {
	return nil
}
