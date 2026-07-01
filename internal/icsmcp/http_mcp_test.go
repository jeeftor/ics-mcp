package icsmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHTTPAPIManagesCalendarsAndServesAdminUI(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	svc.SetBuildInfo(BuildInfo{Version: "v9.9.9", Commit: "abc123", Date: "2026-06-29"})
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	bodyText := string(body)
	for _, want := range []string{"ICS MCP", "Info", "REST", "Calendars", "Meetings", "MCP Tools", "MCP Server", "REST API", "Set Me Up", "HTTP Client Config", "Telegram Outputs", "telegram-quick-links", "telegram-today-agenda", "renderTelegramLinks", "api/free-busy", "Runtime Config", "Build", "Endpoint", "Internal", "External", "endpoint-rows", "Copy", "copyEndpoint", "rest-endpoint-picker", "rest-calendar", "rest-format", "tg-text", "tg-html", "tg-markdownv2", "rest-layout", "rest-fields", "rest-field-options", "rest-time-style", "rest-show-timezone", "rest-rendered-preview", "rest-raw-block", "renderRESTRenderedPreview", "renderMarkdownFragment", "applyRESTHelp", "csv", "summary", "status", "links", "custom", "day date range", "day start", "show timezone", "rest-generated-internal", "rest-generated-external", "run-rest", "open-rest", "renderRESTPreview", "Preview Tool Args", "meeting-tool-args", "run-meeting-preview", "upcoming_meetings_by_calendar/call", "Example URLs", "Next Meetings By Calendar", "meeting-groups", "calendar-meeting-group", "calendar-meeting-header", "meeting-table", "status-column", "time-column", "meta-column", "meeting-badge", "Join", "Ends", "General Queries", "include_in_general_queries", "Save Selection", "general-query-selection", "selectedGeneralCalendarIDs", "tool-name", "tool-description", "json-key", "json-node", "renderJSONNode", "formatMeetingDate", "formatMeetingTime", "formatDuration"} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("admin UI missing %q", want)
		}
	}
	assertOrder(t, bodyText, `data-tab="info"`, `data-tab="calendars"`, `data-tab="meetings"`, `data-tab="tools"`, `data-tab="rest"`)

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	var add Calendar
	doJSON(t, http.MethodPost, server.URL+"/api/calendars", AddCalendarInput{
		Key:  "team",
		Name: "Team",
		URL:  feed.URL,
	}, &add)

	var list []CalendarStatus
	doJSON(t, http.MethodGet, server.URL+"/api/calendars", nil, &list)
	if len(list) != 1 || list[0].Name != "Team" {
		t.Fatalf("calendar list = %#v", list)
	}

	var renamed Calendar
	doJSON(t, http.MethodPatch, server.URL+"/api/calendars/"+add.ID, UpdateCalendarInput{Name: "Renamed"}, &renamed)
	if renamed.Name != "Renamed" {
		t.Fatalf("renamed calendar = %#v", renamed)
	}

	var meetings []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/meetings?limit=10&detail=full", nil, &meetings)
	if len(meetings) != 1 || meetings[0].Name != "Planning" {
		t.Fatalf("meetings preview = %#v", meetings)
	}

	var utcMeetings []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/meetings?limit=10&timezone=UTC&detail=full", nil, &utcMeetings)
	if len(utcMeetings) != 1 || utcMeetings[0].Timezone != "UTC" {
		t.Fatalf("UTC meetings preview = %#v", utcMeetings)
	}

	resp, err = http.Get(server.URL + "/api/meetings?timezone=America%2FDenbver")
	if err != nil {
		t.Fatalf("GET invalid timezone error = %v", err)
	}
	invalidTimezoneBody, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(invalid timezone) error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(invalidTimezoneBody), "America/Denbver") {
		t.Fatalf("invalid timezone response status=%d body=%s", resp.StatusCode, invalidTimezoneBody)
	}

	var groups []CalendarMeetingGroup
	doJSON(t, http.MethodGet, server.URL+"/api/meetings/by-calendar?limit=10&detail=full", nil, &groups)
	if len(groups) != 1 || groups[0].CalendarName != "Renamed" || len(groups[0].Meetings) != 1 {
		t.Fatalf("grouped meetings preview = %#v", groups)
	}

	var tools []ToolInfo
	doJSON(t, http.MethodGet, server.URL+"/api/tools", nil, &tools)
	if len(tools) == 0 || tools[0].Name != "upcoming_meetings" || !containsTool(tools, "upcoming_meetings_by_calendar") {
		t.Fatalf("tools preview = %#v", tools)
	}

	var toolResult ToolCallResponse
	doJSON(t, http.MethodPost, server.URL+"/api/tools/upcoming_meetings/call", ToolCallRequest{
		Arguments: json.RawMessage(`{"limit":10}`),
	}, &toolResult)
	if toolResult.Tool != "upcoming_meetings" {
		t.Fatalf("tool call response = %#v", toolResult)
	}

	var status Status
	doJSON(t, http.MethodGet, server.URL+"/api/status", nil, &status)
	if len(status.Calendars) != 1 || status.Calendars[0].Name != "Renamed" || status.Version.Version != "v9.9.9" {
		t.Fatalf("status = %#v", status)
	}

	var health map[string]any
	doJSON(t, http.MethodGet, server.URL+"/healthz", nil, &health)
	if health["ok"] != true {
		t.Fatalf("healthz = %#v", health)
	}
	var ready map[string]any
	doJSON(t, http.MethodGet, server.URL+"/readyz", nil, &ready)
	if ready["ok"] != true {
		t.Fatalf("readyz = %#v", ready)
	}

	metricsResp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics error = %v", err)
	}
	metricsBody, err := io.ReadAll(metricsResp.Body)
	_ = metricsResp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(metrics) error = %v", err)
	}
	for _, want := range []string{"icsmcp_calendars_total", "icsmcp_calendar_events"} {
		if !strings.Contains(string(metricsBody), want) {
			t.Fatalf("metrics missing %q:\n%s", want, metricsBody)
		}
	}

	doJSON(t, http.MethodDelete, server.URL+"/api/calendars/"+add.ID, nil, nil)
	doJSON(t, http.MethodGet, server.URL+"/api/calendars", nil, &list)
	if len(list) != 0 {
		t.Fatalf("calendar list after delete = %#v", list)
	}

	_, _ = ctx, svc
}

func TestHTTPCalendarGeneralQuerySelection(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	general, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "general", Name: "General", URL: "https://example.test/general.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(general) error = %v", err)
	}
	private, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "private", Name: "Private", URL: "https://example.test/private.ics"})
	if err != nil {
		t.Fatalf("AddCalendar(private) error = %v", err)
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

	var updated Calendar
	doJSON(t, http.MethodPatch, server.URL+"/api/calendars/"+private.ID, UpdateCalendarInput{IncludeInGeneralQueries: ptr(false)}, &updated)
	if updated.IncludeInGeneralQueries {
		t.Fatalf("updated calendar = %#v, want include_in_general_queries false", updated)
	}

	var list []CalendarStatus
	doJSON(t, http.MethodGet, server.URL+"/api/calendars", nil, &list)
	if len(list) != 2 {
		t.Fatalf("calendar list = %#v", list)
	}
	for _, cal := range list {
		if cal.ID == private.ID && cal.IncludeInGeneralQueries {
			t.Fatalf("private calendar status = %#v, want hidden from general queries", cal)
		}
	}

	var defaultMeetings []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/meetings?limit=10&detail=full", nil, &defaultMeetings)
	if got := meetingNames(defaultMeetings); !slices.Equal(got, []string{"General Meeting"}) {
		t.Fatalf("default meeting names = %#v", got)
	}

	var explicitMeetings []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/meetings?limit=10&detail=full&calendar_id="+private.ID, nil, &explicitMeetings)
	if got := meetingNames(explicitMeetings); !slices.Equal(got, []string{"Private Meeting"}) {
		t.Fatalf("explicit meeting names = %#v", got)
	}

	var saved CalendarSelection
	doJSON(t, http.MethodPut, server.URL+"/api/calendars/general-query-selection", CalendarSelection{CalendarIDs: []string{private.ID}}, &saved)
	if !slices.Equal(saved.CalendarIDs, []string{private.ID}) {
		t.Fatalf("saved selection = %#v, want private only", saved)
	}

	var selected CalendarSelection
	doJSON(t, http.MethodGet, server.URL+"/api/calendars/general-query-selection", nil, &selected)
	if !slices.Equal(selected.CalendarIDs, []string{private.ID}) {
		t.Fatalf("selected calendar ids = %#v, want private only", selected.CalendarIDs)
	}

	doJSON(t, http.MethodGet, server.URL+"/api/meetings?limit=10&detail=full", nil, &defaultMeetings)
	if got := meetingNames(defaultMeetings); !slices.Equal(got, []string{"Private Meeting"}) {
		t.Fatalf("default meeting names after bulk selection = %#v", got)
	}
}

func TestHTTPAndMCPCanExplicitlyQueryDisabledCalendars(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "disabled", Name: "Disabled", URL: "https://example.test/disabled.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		ID:    "disabled-1",
		UID:   "disabled-uid",
		Name:  "Disabled Planning",
		Start: now.Add(time.Hour),
		End:   now.Add(2 * time.Hour),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}
	if _, err := svc.UpdateCalendar(ctx, cal.ID, UpdateCalendarInput{Enabled: ptr(false)}); err != nil {
		t.Fatalf("UpdateCalendar(disable) error = %v", err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	var hidden []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/events?calendar_id="+cal.ID+"&detail=full", nil, &hidden)
	if len(hidden) != 0 {
		t.Fatalf("disabled calendar without opt-in = %#v, want none", hidden)
	}

	var events []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/events?calendar_id="+cal.ID+"&include_disabled=true&detail=full", nil, &events)
	if len(events) != 1 || events[0].Name != "Disabled Planning" {
		t.Fatalf("disabled calendar with opt-in = %#v", events)
	}

	resp, err := PreviewToolCall(ctx, svc, "upcoming_meetings", rawJSON(t, UpcomingQuery{Now: now, CalendarIDs: []string{cal.ID}, IncludeDisabled: true, Detail: "full"}))
	if err != nil {
		t.Fatalf("PreviewToolCall(include disabled) error = %v", err)
	}
	out, ok := resp.Result.(meetingsOutput)
	if !ok || len(out.Meetings) != 1 || out.Meetings[0].Name != "Disabled Planning" {
		t.Fatalf("MCP-style disabled calendar query = %#v", resp)
	}
}

func TestHTTPRESTToolRoutesAliasesFormatsAndOpenAPI(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		ID:         "planning",
		UID:        "planning",
		Name:       "Planning",
		Start:      now.Add(time.Hour),
		End:        now.Add(2 * time.Hour),
		MeetingURL: "https://meet.example.test/planning",
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	var rest meetingsOutput
	doJSON(t, http.MethodGet, server.URL+"/api/rest/upcoming_meetings?limit=10&detail=full", nil, &rest)
	if len(rest.Meetings) != 1 || rest.Meetings[0].Name != "Planning" {
		t.Fatalf("rest upcoming meetings = %#v", rest)
	}

	var search meetingsOutput
	doJSON(t, http.MethodGet, server.URL+"/api/rest/search_meetings?query=plan", nil, &search)
	if len(search.Meetings) != 1 || search.Meetings[0].Name != "Planning" {
		t.Fatalf("rest search meetings = %#v", search)
	}

	var updated calendarOutput
	doJSON(t, http.MethodPost, server.URL+"/api/rest/update_calendar", updateInput{ID: cal.ID, Name: "Updated Work"}, &updated)
	if updated.Calendar.Name != "Updated Work" {
		t.Fatalf("rest update = %#v, want renamed calendar", updated)
	}

	var events []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/events?limit=10&detail=full", nil, &events)
	if len(events) != 1 || events[0].Name != "Planning" {
		t.Fatalf("events alias = %#v", events)
	}

	var calendarEvents []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/calendars/"+cal.ID+"/events?limit=10&detail=full", nil, &calendarEvents)
	if len(calendarEvents) != 1 || calendarEvents[0].CalendarID != cal.ID {
		t.Fatalf("calendar events alias = %#v", calendarEvents)
	}

	var today []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/calendars/"+cal.ID+"/today?detail=full", nil, &today)
	if len(today) != 1 || today[0].Name != "Planning" {
		t.Fatalf("calendar today alias = %#v", today)
	}

	mdBody, mdContentType := doText(t, http.MethodGet, server.URL+"/api/events/today.md", nil, "")
	if !strings.Contains(mdContentType, "text/markdown") || !strings.Contains(mdBody, "# Meetings") || !strings.Contains(mdBody, "| When | Calendar | Title | Duration |") || !strings.Contains(mdBody, "Planning") {
		t.Fatalf("markdown response content-type=%q body=%s", mdContentType, mdBody)
	}

	txtBody, txtContentType := doText(t, http.MethodGet, server.URL+"/api/events/next?format=txt", nil, "")
	if !strings.Contains(txtContentType, "text/plain") || !strings.Contains(txtBody, "Planning") {
		t.Fatalf("text response content-type=%q body=%s", txtContentType, txtBody)
	}

	htmlBody, htmlContentType := doText(t, http.MethodGet, server.URL+"/api/events/by-calendar", nil, "text/html")
	if !strings.Contains(htmlContentType, "text/html") || !strings.Contains(htmlBody, "<html") || !strings.Contains(htmlBody, "<table>") || !strings.Contains(htmlBody, "Planning") {
		t.Fatalf("html response content-type=%q body=%s", htmlContentType, htmlBody)
	}

	asciiBody, asciiContentType := doText(t, http.MethodGet, server.URL+"/api/free-busy.ascii", nil, "")
	if !strings.Contains(asciiContentType, "text/plain") || !strings.Contains(asciiBody, "+") || !strings.Contains(asciiBody, "When") || !strings.Contains(asciiBody, "Calendar") || !strings.Contains(asciiBody, "Duration") || !strings.Contains(asciiBody, "Work") {
		t.Fatalf("ascii response content-type=%q body=%s", asciiContentType, asciiBody)
	}

	csvBody, csvContentType := doText(t, http.MethodGet, server.URL+"/api/events.csv?fields=when,title", nil, "")
	if !strings.Contains(csvContentType, "text/csv") || !strings.Contains(csvBody, "when,title") || !strings.Contains(csvBody, "Planning") || strings.Contains(csvBody, "calendar") {
		t.Fatalf("csv response content-type=%q body=%s", csvContentType, csvBody)
	}
	if strings.Contains(csvBody, "UTC") {
		t.Fatalf("csv response should hide timezone by default: %s", csvBody)
	}

	startOnlyBody, _ := doText(t, http.MethodGet, server.URL+"/api/events.csv?fields=when,title&time_style=start", nil, "")
	if !strings.Contains(startOnlyBody, "Mon 1:00 PM,Planning") || strings.Contains(startOnlyBody, "2:00") || strings.Contains(startOnlyBody, "Jun 29") {
		t.Fatalf("start-only csv response body=%s", startOnlyBody)
	}

	withTimezoneBody, _ := doText(t, http.MethodGet, server.URL+"/api/events.csv?fields=when,title&show_timezone=true", nil, "")
	if !strings.Contains(withTimezoneBody, "UTC") {
		t.Fatalf("csv response should include timezone when requested: %s", withTimezoneBody)
	}

	var spec map[string]any
	doJSON(t, http.MethodGet, server.URL+"/openapi.json", nil, &spec)
	paths, ok := spec["paths"].(map[string]any)
	if !ok {
		t.Fatalf("openapi paths = %#v", spec["paths"])
	}
	for _, want := range []string{"/api/rest/{tool_name}", "/api/events", "/api/events/today", "/api/free-busy", "/api/calendars/{calendar}/events"} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("openapi paths missing %q: %#v", want, paths)
		}
	}
}

func TestHTTPAPIValidatesCalendarFeed(t *testing.T) {
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 13, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	var result ValidateCalendarResult
	doJSON(t, http.MethodPost, server.URL+"/api/calendars/validate", ValidateCalendarInput{URL: feed.URL, Limit: 3}, &result)
	if !result.OK || result.EventCount != 1 || len(result.Meetings) != 1 || result.Meetings[0].Name != "Planning" {
		t.Fatalf("validation result = %#v", result)
	}
}

func TestHTTPAPIValidateCalendarReportsFailures(t *testing.T) {
	svc := newTestService(t)
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/calendars/validate", "application/json", bytes.NewBufferString(`{"url":""}`))
	if err != nil {
		t.Fatalf("POST validate empty URL error = %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(empty URL) error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), "calendar URL is required") {
		t.Fatalf("empty URL response status=%d body=%s", resp.StatusCode, body)
	}

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer feed.Close()
	resp, err = http.Post(server.URL+"/api/calendars/validate", "application/json", bytes.NewBufferString(`{"url":"`+feed.URL+`"}`))
	if err != nil {
		t.Fatalf("POST validate non-2xx error = %v", err)
	}
	body, err = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(non-2xx) error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), "status 404") {
		t.Fatalf("non-2xx response status=%d body=%s", resp.StatusCode, body)
	}
}

func TestHTTPAPIReportsBadRequestsAndMethodErrors(t *testing.T) {
	svc := newTestService(t)
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	for _, tc := range []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "health method", method: http.MethodPost, path: "/healthz", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "ready method", method: http.MethodPost, path: "/readyz", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "metrics method", method: http.MethodPost, path: "/metrics", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "status method", method: http.MethodPost, path: "/api/status", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "meetings method", method: http.MethodPost, path: "/api/meetings", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "meetings invalid limit", method: http.MethodGet, path: "/api/meetings?limit=bogus", wantStatus: http.StatusBadRequest, wantBody: "invalid syntax"},
		{name: "meetings invalid lookahead", method: http.MethodGet, path: "/api/meetings?lookahead_days=bogus", wantStatus: http.StatusBadRequest, wantBody: "invalid syntax"},
		{name: "meetings invalid description max", method: http.MethodGet, path: "/api/meetings?description_max_chars=bogus", wantStatus: http.StatusBadRequest, wantBody: "invalid syntax"},
		{name: "meetings invalid after", method: http.MethodGet, path: "/api/meetings?after=not-a-time", wantStatus: http.StatusBadRequest, wantBody: "cannot parse"},
		{name: "meetings invalid before", method: http.MethodGet, path: "/api/meetings?before=not-a-time", wantStatus: http.StatusBadRequest, wantBody: "cannot parse"},
		{name: "grouped meetings method", method: http.MethodPost, path: "/api/meetings/by-calendar", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "grouped meetings invalid limit", method: http.MethodGet, path: "/api/meetings/by-calendar?limit=bogus", wantStatus: http.StatusBadRequest, wantBody: "invalid syntax"},
		{name: "tools list method", method: http.MethodPost, path: "/api/tools", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "calendar collection method", method: http.MethodPut, path: "/api/calendars", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "calendar add bad json", method: http.MethodPost, path: "/api/calendars", body: "{", wantStatus: http.StatusBadRequest, wantBody: "unexpected EOF"},
		{name: "calendar selection method", method: http.MethodPost, path: "/api/calendars/general-query-selection", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "calendar selection bad json", method: http.MethodPut, path: "/api/calendars/general-query-selection", body: "{", wantStatus: http.StatusBadRequest, wantBody: "unexpected EOF"},
		{name: "calendar validate method", method: http.MethodGet, path: "/api/calendars/validate", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "calendar validate bad json", method: http.MethodPost, path: "/api/calendars/validate", body: "{", wantStatus: http.StatusBadRequest, wantBody: "unexpected EOF"},
		{name: "calendar empty id", method: http.MethodGet, path: "/api/calendars/", wantStatus: http.StatusNotFound, wantBody: "404 page not found"},
		{name: "calendar patch bad json", method: http.MethodPatch, path: "/api/calendars/missing", body: "{", wantStatus: http.StatusBadRequest, wantBody: "unexpected EOF"},
		{name: "calendar item unsupported method", method: http.MethodPost, path: "/api/calendars/missing", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "calendar refresh method", method: http.MethodGet, path: "/api/calendars/missing/refresh", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "tools bad json", method: http.MethodPost, path: "/api/tools/upcoming_meetings/call", body: "{", wantStatus: http.StatusBadRequest, wantBody: "unexpected EOF"},
		{name: "tools method", method: http.MethodGet, path: "/api/tools/upcoming_meetings/call", wantStatus: http.StatusMethodNotAllowed, wantBody: "feature not supported"},
		{name: "unknown admin path", method: http.MethodGet, path: "/nope", wantStatus: http.StatusNotFound, wantBody: "404 page not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, server.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s %s error = %v", tc.method, tc.path, err)
			}
			data, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if resp.StatusCode != tc.wantStatus || !strings.Contains(string(data), tc.wantBody) {
				t.Fatalf("%s %s status=%d body=%s, want status=%d containing %q", tc.method, tc.path, resp.StatusCode, data, tc.wantStatus, tc.wantBody)
			}
		})
	}

	resp, err := http.Post(server.URL+"/api/tools/missing_tool/call", "application/json", strings.NewReader(`{"arguments":{}}`))
	if err != nil {
		t.Fatalf("POST unknown tool error = %v", err)
	}
	data, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("ReadAll(unknown tool) error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(data), `unknown tool \"missing_tool\"`) {
		t.Fatalf("unknown tool status=%d body=%s", resp.StatusCode, data)
	}

	notFoundResp, err := http.Post(server.URL+"/api/tools/upcoming_meetings/nope", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST non-call tool route error = %v", err)
	}
	_ = notFoundResp.Body.Close()
	if notFoundResp.StatusCode != http.StatusNotFound {
		t.Fatalf("non-call tool route status = %d, want 404", notFoundResp.StatusCode)
	}
}

func TestHTTPAPIReadinessAndMetricsReportStoreErrors(t *testing.T) {
	svc := newTestService(t)
	if err := svc.store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	for _, tc := range []struct {
		name string
		path string
	}{
		{name: "readiness", path: "/readyz"},
		{name: "metrics", path: "/metrics"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s error = %v", tc.path, err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil {
				t.Fatalf("ReadAll(%s) error = %v", tc.path, err)
			}
			if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), "database is closed") {
				t.Fatalf("GET %s status=%d body=%s, want 500 database is closed", tc.path, resp.StatusCode, body)
			}
		})
	}
}

func TestParseBoolQueryAcceptedValues(t *testing.T) {
	for _, value := range []string{"1", "true", "TRUE", " yes ", "on"} {
		if !parseBoolQuery(value) {
			t.Fatalf("parseBoolQuery(%q) = false, want true", value)
		}
	}
	for _, value := range []string{"", "0", "false", "no", "off", "anything"} {
		if parseBoolQuery(value) {
			t.Fatalf("parseBoolQuery(%q) = true, want false", value)
		}
	}
}

func TestWriteJSONReportsEncodeFailures(t *testing.T) {
	recorder := httptest.NewRecorder()

	writeJSON(recorder, map[string]any{"bad": make(chan int)}, nil)

	resp := recorder.Result()
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("writeJSON() status = %d body=%s, want 500", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "unsupported type: chan int") {
		t.Fatalf("writeJSON() body = %q, want encoder error", body)
	}
}

func TestUpcomingQueryFromRequestParsesAllSupportedFilters(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/meetings?limit=7&lookahead_days=14&calendar_id=work&calendar_id=home&calendar=side&query=plan&range=today_tomorrow&timezone=America%2FDenver&detail=full&format=tg-html&sort=agenda&in_progress_only=yes&exclude_all_day=on&exclude_cancelled=true&include_description=1&include_links=false&links_only=true&include_disabled=true&description_max_chars=42&after=2026-06-29T15:00:00Z&before=2026-06-30T15:00:00Z", nil)

	query, err := upcomingQueryFromRequest(req)
	if err != nil {
		t.Fatalf("upcomingQueryFromRequest() error = %v", err)
	}
	if query.Limit != 7 || query.LookaheadDays != 14 || query.Query != "plan" || query.Window != "today_tomorrow" || query.Timezone != "America/Denver" || query.Detail != "full" || query.Format != "tg-html" || query.Sort != "agenda" {
		t.Fatalf("basic query fields = %#v", query)
	}
	if !slices.Equal(query.CalendarIDs, []string{"work", "home", "side"}) {
		t.Fatalf("calendar ids = %#v", query.CalendarIDs)
	}
	if !query.InProgressOnly || !query.ExcludeAllDay || !query.ExcludeCancelled || !query.IncludeDescription || !query.IncludeDisabled {
		t.Fatalf("boolean filters = %#v", query)
	}
	if query.IncludeLinks == nil || *query.IncludeLinks || !query.LinksOnly {
		t.Fatalf("link filters = %#v", query)
	}
	if query.DescriptionMaxChars != 42 {
		t.Fatalf("description max chars = %d, want 42", query.DescriptionMaxChars)
	}
	if got := query.After.Format(time.RFC3339); got != "2026-06-29T15:00:00Z" {
		t.Fatalf("after = %s", got)
	}
	if got := query.Before.Format(time.RFC3339); got != "2026-06-30T15:00:00Z" {
		t.Fatalf("before = %s", got)
	}
}

func TestHTTPMeetingAPIFormatsTelegramText(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		CalendarID:     cal.ID,
		CalendarName:   cal.Name,
		UID:            "planning",
		Name:           "Planning & Review",
		MeetingURL:     "https://meet.example.test/planning?x=1&y=2",
		MeetingURLType: "meet",
		Start:          time.Date(2026, 6, 29, 13, 0, 0, 0, time.UTC),
		End:            time.Date(2026, 6, 29, 14, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/meetings?limit=10&timezone=UTC&format=tg-text")
	if err != nil {
		t.Fatalf("GET formatted meetings error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	for _, want := range []string{"Mon Jun 29", "- 1:00-2:00 PM Planning & Review", "Join: https://meet.example.test/planning?x=1&y=2"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("formatted body missing %q:\n%s", want, body)
		}
	}
}

func TestHTTPGroupedMeetingAPIFormatsTelegramHTML(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work & Personal", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		CalendarID:   cal.ID,
		CalendarName: cal.Name,
		UID:          "planning",
		Name:         "Planning <Review>",
		Start:        time.Date(2026, 6, 29, 13, 0, 0, 0, time.UTC),
		End:          time.Date(2026, 6, 29, 14, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/meetings/by-calendar?limit=10&timezone=UTC&format=tg-html")
	if err != nil {
		t.Fatalf("GET formatted grouped meetings error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.StatusCode, body)
	}
	for _, want := range []string{"<b>Work &amp; Personal</b>", "<b>Mon Jun 29</b>", "Planning &lt;Review&gt;"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("formatted grouped body missing %q:\n%s", want, body)
		}
	}
}

func TestHTTPFreeBusyAPIFormatsTelegramText(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		CalendarID:   cal.ID,
		CalendarName: cal.Name,
		UID:          "planning",
		Name:         "Private Planning",
		Start:        time.Date(2026, 6, 29, 13, 0, 0, 0, time.UTC),
		End:          time.Date(2026, 6, 29, 14, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/free-busy?limit=10&timezone=UTC&format=tg-text")
	if err != nil {
		t.Fatalf("GET formatted free-busy error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", resp.StatusCode, body)
	}
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", got)
	}
	for _, want := range []string{"Busy", "- Mon Jun 29 1:00-2:00 PM UTC (1 hr)", "Work"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("formatted free-busy body missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(string(body), "Private Planning") {
		t.Fatalf("formatted free-busy leaked meeting title:\n%s", body)
	}

	var jsonOut freeBusyOutput
	doJSON(t, http.MethodGet, server.URL+"/api/free-busy?limit=10&timezone=UTC", nil, &jsonOut)
	if len(jsonOut.Busy) != 1 || jsonOut.Busy[0].Calendar != "Work" || jsonOut.Text != "" {
		t.Fatalf("free-busy JSON = %#v", jsonOut)
	}
}

func TestHTTPMeetingAPIRejectsUnsupportedFormat(t *testing.T) {
	svc := newTestService(t)
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/meetings?format=bad-format")
	if err != nil {
		t.Fatalf("GET unsupported formatted meetings error = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), `unsupported meeting format`) || !strings.Contains(string(body), `bad-format`) {
		t.Fatalf("body = %s, want unsupported format error", body)
	}
}

func TestToolPreviewFormattedMeetingsKeepStructuredFieldsAndText(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		CalendarID:     cal.ID,
		CalendarName:   cal.Name,
		UID:            "planning",
		Name:           "Planning & Review",
		MeetingURL:     "https://meet.example.test/planning?x=1&y=2",
		MeetingURLType: "meet",
		Start:          time.Date(2026, 6, 29, 13, 0, 0, 0, time.UTC),
		End:            time.Date(2026, 6, 29, 14, 0, 0, 0, time.UTC),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}

	resp, err := PreviewToolCall(ctx, svc, "today_meetings", json.RawMessage(`{"timezone":"UTC","format":"tg-html"}`))
	if err != nil {
		t.Fatalf("today_meetings preview error = %v", err)
	}
	out, ok := resp.Result.(meetingsOutput)
	if !ok {
		t.Fatalf("today_meetings result type = %T", resp.Result)
	}
	if len(out.Meetings) != 1 || out.Meetings[0].Name != "Planning & Review" {
		t.Fatalf("structured meetings = %#v", out.Meetings)
	}
	if out.Text == "" {
		t.Fatalf("formatted text is empty: %#v", out)
	}
	for _, want := range []string{"<b>Mon Jun 29</b>", "Planning &amp; Review", `<a href="https://meet.example.test/planning?x=1&amp;y=2">Join</a>`} {
		if !strings.Contains(out.Text, want) {
			t.Fatalf("formatted text missing %q:\n%s", want, out.Text)
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, want := range []string{"meetings", "text"} {
		if _, ok := raw[want]; !ok {
			t.Fatalf("formatted output missing raw JSON field %q: %s", want, data)
		}
	}
}

func TestUpcomingQueryFromRequestSupportsLegacyOnlyOngoingFilter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/meetings?only_ongoing=yes", nil)

	query, err := upcomingQueryFromRequest(req)
	if err != nil {
		t.Fatalf("upcomingQueryFromRequest() error = %v", err)
	}
	if !query.InProgressOnly {
		t.Fatalf("in_progress_only = false for legacy only_ongoing query: %#v", query)
	}
}

func TestHTTPAPIAddCalendarRefreshesImmediately(t *testing.T) {
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	var add Calendar
	doJSON(t, http.MethodPost, server.URL+"/api/calendars", AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL}, &add)

	var meetings []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/meetings?limit=10&detail=full", nil, &meetings)
	if len(meetings) != 1 || meetings[0].Name != "Planning" {
		t.Fatalf("meetings after add = %#v", meetings)
	}
	var statuses []CalendarStatus
	doJSON(t, http.MethodGet, server.URL+"/api/calendars", nil, &statuses)
	if len(statuses) != 1 || statuses[0].EventCount != 1 || statuses[0].LastSuccess == nil {
		t.Fatalf("status after add = %#v", statuses)
	}
}

func TestHTTPAPIRefreshCalendarRouteUpdatesCache(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	var ok map[string]bool
	doJSON(t, http.MethodPost, server.URL+"/api/calendars/"+cal.ID+"/refresh", nil, &ok)
	if !ok["ok"] {
		t.Fatalf("refresh response = %#v, want ok", ok)
	}

	var statuses []CalendarStatus
	doJSON(t, http.MethodGet, server.URL+"/api/calendars", nil, &statuses)
	if len(statuses) != 1 || statuses[0].EventCount != 1 || statuses[0].LastSuccess == nil {
		t.Fatalf("status after manual refresh = %#v", statuses)
	}
	var meetings []Meeting
	doJSON(t, http.MethodGet, server.URL+"/api/meetings?limit=10&detail=full", nil, &meetings)
	if len(meetings) != 1 || meetings[0].Name != "Planning" {
		t.Fatalf("meetings after manual refresh = %#v", meetings)
	}
}

func TestToolPreviewRejectsInvalidTimezone(t *testing.T) {
	svc := newTestService(t)
	_, err := PreviewToolCall(context.Background(), svc, "upcoming_meetings", json.RawMessage(`{"timezone":"America/Denbver"}`))
	if err == nil || !strings.Contains(err.Error(), "America/Denbver") {
		t.Fatalf("PreviewToolCall invalid timezone error = %v, want timezone error", err)
	}
}

func TestUpcomingQueryUnmarshalJSONSupportsCurrentAndLegacyInProgressFilters(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "current field", raw: `{"in_progress_only":true}`, want: true},
		{name: "legacy field", raw: `{"only_ongoing":true}`, want: true},
		{name: "current field wins true with legacy false", raw: `{"in_progress_only":true,"only_ongoing":false}`, want: true},
		{name: "legacy field upgrades current false", raw: `{"in_progress_only":false,"only_ongoing":true}`, want: true},
		{name: "both false", raw: `{"in_progress_only":false,"only_ongoing":false}`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var query UpcomingQuery
			if err := json.Unmarshal([]byte(tt.raw), &query); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			if query.InProgressOnly != tt.want {
				t.Fatalf("InProgressOnly = %v, want %v", query.InProgressOnly, tt.want)
			}
		})
	}

	var query UpcomingQuery
	if err := json.Unmarshal([]byte(`{"limit":"bad"}`), &query); err == nil {
		t.Fatalf("json.Unmarshal(invalid) error = nil, want error")
	}
}

func TestDecodeToolArgsDefaultsEmptyRawMessageToObject(t *testing.T) {
	var got struct {
		Limit int `json:"limit"`
	}

	if err := decodeToolArgs(nil, &got); err != nil {
		t.Fatalf("decodeToolArgs(nil) error = %v", err)
	}
	if got.Limit != 0 {
		t.Fatalf("decodeToolArgs(nil) = %#v, want zero-value struct", got)
	}
}

func TestToolPreviewExecutesReadAndAdminTools(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 13, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	addResp, err := PreviewToolCall(ctx, svc, "add_calendar", rawJSON(t, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL}))
	if err != nil {
		t.Fatalf("add_calendar preview error = %v", err)
	}
	addOut, ok := addResp.Result.(calendarOutput)
	if !ok || addOut.Calendar.ID == "" || addOut.Calendar.Name != "Work" {
		t.Fatalf("add_calendar preview = %#v", addResp)
	}

	for _, tc := range []struct {
		name string
		args json.RawMessage
		want string
	}{
		{name: "upcoming_meetings", args: json.RawMessage(`{"limit":10}`), want: "Planning"},
		{name: "next_meeting", args: json.RawMessage(`{}`), want: "Planning"},
		{name: "next_meetings", args: json.RawMessage(`{}`), want: "Planning"},
		{name: "today_meetings", args: json.RawMessage(`{}`), want: "Planning"},
		{name: "current_meetings", args: json.RawMessage(`{}`), want: "Planning"},
		{name: "search_meetings", args: json.RawMessage(`{"query":"plan"}`), want: "Planning"},
	} {
		resp, err := PreviewToolCall(ctx, svc, tc.name, tc.args)
		if err != nil {
			t.Fatalf("%s preview error = %v", tc.name, err)
		}
		out, ok := resp.Result.(meetingsOutput)
		if !ok || len(out.Meetings) != 1 || out.Meetings[0].Name != tc.want {
			t.Fatalf("%s preview = %#v", tc.name, resp)
		}
	}

	busyResp, err := PreviewToolCall(ctx, svc, "free_busy", json.RawMessage(`{"limit":10}`))
	if err != nil {
		t.Fatalf("free_busy preview error = %v", err)
	}
	busyOut, ok := busyResp.Result.(freeBusyOutput)
	if !ok || len(busyOut.Busy) != 1 || busyOut.Busy[0].When == "" || busyOut.Busy[0].DurationMinutes != 60 {
		t.Fatalf("free_busy preview = %#v", busyResp)
	}

	formattedBusyResp, err := PreviewToolCall(ctx, svc, "free_busy", json.RawMessage(`{"limit":10,"format":"tg-text"}`))
	if err != nil {
		t.Fatalf("formatted free_busy preview error = %v", err)
	}
	formattedBusyOut, ok := formattedBusyResp.Result.(freeBusyOutput)
	if !ok || len(formattedBusyOut.Busy) != 1 || !strings.Contains(formattedBusyOut.Text, "Busy") || !strings.Contains(formattedBusyOut.Text, "1 hr") {
		t.Fatalf("formatted free_busy preview = %#v", formattedBusyResp)
	}

	groupResp, err := PreviewToolCall(ctx, svc, "upcoming_meetings_by_calendar", json.RawMessage(`{"limit":10}`))
	if err != nil {
		t.Fatalf("upcoming_meetings_by_calendar preview error = %v", err)
	}
	groupOut, ok := groupResp.Result.(groupedMeetingsOutput)
	if !ok || len(groupOut.Calendars) != 1 || groupOut.Calendars[0].CalendarName != "Work" {
		t.Fatalf("upcoming_meetings_by_calendar preview = %#v", groupResp)
	}

	statusResp, err := PreviewToolCall(ctx, svc, "server_status", nil)
	if err != nil {
		t.Fatalf("server_status preview error = %v", err)
	}
	statusOut, ok := statusResp.Result.(statusOutput)
	if !ok || len(statusOut.Status.Calendars) != 1 {
		t.Fatalf("server_status preview = %#v", statusResp)
	}

	listResp, err := PreviewToolCall(ctx, svc, "list_calendars", nil)
	if err != nil {
		t.Fatalf("list_calendars preview error = %v", err)
	}
	listOut, ok := listResp.Result.(calendarsOutput)
	if !ok || len(listOut.Calendars) != 1 {
		t.Fatalf("list_calendars preview = %#v", listResp)
	}

	refreshResp, err := PreviewToolCall(ctx, svc, "refresh_calendar", rawJSON(t, refreshInput{ID: addOut.Calendar.ID}))
	if err != nil {
		t.Fatalf("refresh_calendar preview error = %v", err)
	}
	refreshOut, ok := refreshResp.Result.(okOutput)
	if !ok || !refreshOut.OK {
		t.Fatalf("refresh_calendar preview = %#v", refreshResp)
	}

	refreshAllResp, err := PreviewToolCall(ctx, svc, "refresh_all_calendars", nil)
	if err != nil {
		t.Fatalf("refresh_all_calendars preview error = %v", err)
	}
	refreshAllOut, ok := refreshAllResp.Result.(refreshAllOutput)
	if !ok || len(refreshAllOut.Results) != 1 || !refreshAllOut.Results[0].OK {
		t.Fatalf("refresh_all_calendars preview = %#v", refreshAllResp)
	}

	validateResp, err := PreviewToolCall(ctx, svc, "validate_calendar", rawJSON(t, ValidateCalendarInput{URL: feed.URL, Limit: 1}))
	if err != nil {
		t.Fatalf("validate_calendar preview error = %v", err)
	}
	validateOut, ok := validateResp.Result.(ValidateCalendarResult)
	if !ok || !validateOut.OK || validateOut.EventCount != 1 {
		t.Fatalf("validate_calendar preview = %#v", validateResp)
	}

	updateResp, err := PreviewToolCall(ctx, svc, "update_calendar", rawJSON(t, updateInput{ID: addOut.Calendar.ID, Name: "Renamed", IncludeInGeneralQueries: ptr(false)}))
	if err != nil {
		t.Fatalf("update_calendar preview error = %v", err)
	}
	updateOut, ok := updateResp.Result.(calendarOutput)
	if !ok || updateOut.Calendar.Name != "Renamed" || updateOut.Calendar.IncludeInGeneralQueries {
		t.Fatalf("update_calendar preview = %#v", updateResp)
	}

	removeResp, err := PreviewToolCall(ctx, svc, "remove_calendar", rawJSON(t, removeInput{ID: addOut.Calendar.ID}))
	if err != nil {
		t.Fatalf("remove_calendar preview error = %v", err)
	}
	removeOut, ok := removeResp.Result.(okOutput)
	if !ok || !removeOut.OK {
		t.Fatalf("remove_calendar preview = %#v", removeResp)
	}
	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 0 {
		t.Fatalf("calendars after preview remove = %#v", calendars)
	}
}

func TestToolPreviewReadToolDefaultArgumentsAreExecutable(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 13, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()
	if _, err := PreviewToolCall(ctx, svc, "add_calendar", rawJSON(t, AddCalendarInput{Key: "work", Name: "Work", URL: feed.URL})); err != nil {
		t.Fatalf("add_calendar preview error = %v", err)
	}

	for _, tool := range ToolInfos() {
		if tool.Category != "read" || !tool.ReadOnly {
			continue
		}
		t.Run(tool.Name, func(t *testing.T) {
			if _, err := PreviewToolCall(ctx, svc, tool.Name, rawJSON(t, tool.DefaultArguments)); err != nil {
				t.Fatalf("PreviewToolCall(%s defaults) error = %v", tool.Name, err)
			}
		})
	}
}

func TestToolPreviewMeetingPresetsApplyFilters(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{
		{ID: "current", UID: "current", Name: "Current Planning", Start: now.Add(-30 * time.Minute), End: now.Add(30 * time.Minute)},
		{ID: "future", UID: "future", Name: "Future Planning", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
		{ID: "all-day", UID: "all-day", Name: "All Day Hold", Start: now.Add(3 * time.Hour), End: now.Add(27 * time.Hour), AllDay: true},
		{ID: "cancelled", UID: "cancelled", Name: "Cancelled Planning", Start: now.Add(4 * time.Hour), End: now.Add(5 * time.Hour), Cancelled: true},
	}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}

	nextResp, err := PreviewToolCall(ctx, svc, "next_meetings", json.RawMessage(`{"limit":10}`))
	if err != nil {
		t.Fatalf("next_meetings preview error = %v", err)
	}
	nextOut, ok := nextResp.Result.(meetingsOutput)
	if !ok {
		t.Fatalf("next_meetings result type = %T", nextResp.Result)
	}
	if got := meetingNames(nextOut.Meetings); !slices.Equal(got, []string{"Current Planning", "Future Planning"}) {
		t.Fatalf("next_meetings names = %#v, want current and future non-all-day non-cancelled meetings", got)
	}

	currentResp, err := PreviewToolCall(ctx, svc, "current_meetings", json.RawMessage(`{"limit":10}`))
	if err != nil {
		t.Fatalf("current_meetings preview error = %v", err)
	}
	currentOut, ok := currentResp.Result.(meetingsOutput)
	if !ok {
		t.Fatalf("current_meetings result type = %T", currentResp.Result)
	}
	if got := meetingNames(currentOut.Meetings); !slices.Equal(got, []string{"Current Planning"}) {
		t.Fatalf("current_meetings names = %#v, want only ongoing meeting", got)
	}

	legacyResp, err := PreviewToolCall(ctx, svc, "upcoming_meetings", json.RawMessage(`{"limit":10,"only_ongoing":true}`))
	if err != nil {
		t.Fatalf("upcoming_meetings legacy only_ongoing preview error = %v", err)
	}
	legacyOut, ok := legacyResp.Result.(meetingsOutput)
	if !ok {
		t.Fatalf("upcoming_meetings legacy result type = %T", legacyResp.Result)
	}
	if got := meetingNames(legacyOut.Meetings); !slices.Equal(got, []string{"Current Planning"}) {
		t.Fatalf("upcoming_meetings legacy only_ongoing names = %#v, want only in-progress meeting", got)
	}

	searchResp, err := PreviewToolCall(ctx, svc, "search_meetings", json.RawMessage(`{"query":"future","limit":10}`))
	if err != nil {
		t.Fatalf("search_meetings preview error = %v", err)
	}
	searchOut, ok := searchResp.Result.(meetingsOutput)
	if !ok {
		t.Fatalf("search_meetings result type = %T", searchResp.Result)
	}
	if got := meetingNames(searchOut.Meetings); !slices.Equal(got, []string{"Future Planning"}) {
		t.Fatalf("search_meetings names = %#v, want query-matched meeting", got)
	}
}

func TestToolPreviewReportsDecodeAndUnknownToolErrors(t *testing.T) {
	svc := newTestService(t)
	if _, err := PreviewToolCall(context.Background(), svc, "upcoming_meetings", json.RawMessage(`{`)); err == nil || !strings.Contains(err.Error(), "decode tool arguments") {
		t.Fatalf("invalid JSON error = %v, want decode error", err)
	}
	if _, err := PreviewToolCall(context.Background(), svc, "missing_tool", nil); err == nil || !strings.Contains(err.Error(), `unknown tool "missing_tool"`) {
		t.Fatalf("unknown tool error = %v, want unknown tool error", err)
	}
}

func TestToolPreviewReportsDecodeErrorsForArgumentTools(t *testing.T) {
	svc := newTestService(t)
	for _, name := range []string{
		"upcoming_meetings",
		"upcoming_meetings_by_calendar",
		"next_meeting",
		"next_meetings",
		"today_meetings",
		"current_meetings",
		"search_meetings",
		"free_busy",
		"add_calendar",
		"validate_calendar",
		"update_calendar",
		"remove_calendar",
		"refresh_calendar",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := PreviewToolCall(context.Background(), svc, name, json.RawMessage(`{`))
			if err == nil || !strings.Contains(err.Error(), "decode tool arguments") {
				t.Fatalf("%s decode error = %v, want decode tool arguments", name, err)
			}
		})
	}
}

func TestHTTPAPIEmptyCollectionsEncodeAsArrays(t *testing.T) {
	svc := newTestService(t)
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	for _, path := range []string{"/api/calendars", "/api/meetings", "/api/meetings/by-calendar"} {
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s error = %v", path, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("ReadAll(%s) error = %v", path, err)
		}
		if strings.TrimSpace(string(body)) != "[]" {
			t.Fatalf("GET %s body = %s, want []", path, body)
		}
	}

	var status Status
	doJSON(t, http.MethodGet, server.URL+"/api/status", nil, &status)
	if status.Calendars == nil {
		t.Fatalf("status calendars = nil, want empty slice")
	}
}

func TestCombinedHTTPHandlerServesMCPEndpoint(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("Connect(/mcp) error = %v", err)
	}
	defer session.Close()

	var toolNames []string
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		toolNames = append(toolNames, tool.Name)
	}
	if !contains(toolNames, "upcoming_meetings") {
		t.Fatalf("/mcp tool names = %#v, missing upcoming_meetings", toolNames)
	}
}

func TestMCPResourcesAndPromptsExposeCalendarContext(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 13, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })
	cal, err := svc.AddCalendar(ctx, AddCalendarInput{Key: "work", Name: "Work", URL: "https://example.test/work.ics"})
	if err != nil {
		t.Fatalf("AddCalendar() error = %v", err)
	}
	if err := svc.ReplaceEvents(ctx, cal.ID, []EventInstance{{
		CalendarID:   cal.ID,
		CalendarName: cal.Name,
		Name:         "Planning",
		Start:        now.Add(time.Hour),
		End:          now.Add(90 * time.Minute),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp"}, nil)
	if err != nil {
		t.Fatalf("Connect(/mcp) error = %v", err)
	}
	defer session.Close()

	resources, err := session.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources() error = %v", err)
	}
	var resourceURIs []string
	for _, resource := range resources.Resources {
		resourceURIs = append(resourceURIs, resource.URI)
	}
	for _, want := range []string{"icsmcp://status", "icsmcp://calendars", "icsmcp://meetings/today", "icsmcp://meetings/upcoming"} {
		if !contains(resourceURIs, want) {
			t.Fatalf("resource URIs = %#v, missing %s", resourceURIs, want)
		}
	}

	read, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "icsmcp://meetings/today"})
	if err != nil {
		t.Fatalf("ReadResource(today) error = %v", err)
	}
	if len(read.Contents) != 1 || read.Contents[0].MIMEType != "application/json" || !strings.Contains(read.Contents[0].Text, "Planning") {
		t.Fatalf("today resource = %#v", read.Contents)
	}

	prompts, err := session.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts() error = %v", err)
	}
	var promptNames []string
	for _, prompt := range prompts.Prompts {
		promptNames = append(promptNames, prompt.Name)
	}
	for _, want := range []string{"daily_briefing", "meeting_prep", "availability_summary", "calendar_debug_report"} {
		if !contains(promptNames, want) {
			t.Fatalf("prompt names = %#v, missing %s", promptNames, want)
		}
	}

	prompt, err := session.GetPrompt(ctx, &mcp.GetPromptParams{Name: "daily_briefing"})
	if err != nil {
		t.Fatalf("GetPrompt(daily_briefing) error = %v", err)
	}
	if len(prompt.Messages) != 1 {
		t.Fatalf("daily_briefing messages = %#v", prompt.Messages)
	}
	content, ok := prompt.Messages[0].Content.(*mcp.TextContent)
	if !ok || !strings.Contains(content.Text, "today_meetings") || !strings.Contains(content.Text, "icsmcp://meetings/today") {
		t.Fatalf("daily_briefing prompt content = %#v", prompt.Messages[0].Content)
	}
}

func TestMCPToolsExposeMeetingsAndAdminMutations(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 13, 30, 0, 0, time.UTC)
	svc.SetClock(func() time.Time { return now })

	mcpServer := NewMCPServer(svc)
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	defer httpServer.Close()

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v0.0.1"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL}, nil)
	if err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	defer session.Close()

	var toolNames []string
	var upcomingSchema any
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		toolNames = append(toolNames, tool.Name)
		if tool.Name == "upcoming_meetings" {
			upcomingSchema = tool.InputSchema
		}
	}
	for _, want := range []string{
		"upcoming_meetings",
		"upcoming_meetings_by_calendar",
		"next_meeting",
		"next_meetings",
		"today_meetings",
		"current_meetings",
		"search_meetings",
		"free_busy",
		"server_status",
		"list_calendars",
		"add_calendar",
		"update_calendar",
		"remove_calendar",
		"refresh_calendar",
		"refresh_all_calendars",
		"validate_calendar",
	} {
		if !contains(toolNames, want) {
			t.Fatalf("tool names = %#v, missing %s", toolNames, want)
		}
	}
	schemaData, err := json.Marshal(upcomingSchema)
	if err != nil {
		t.Fatalf("Marshal upcoming tool schema error = %v", err)
	}
	for _, want := range []string{"limit", "calendar_ids", "lookahead_days", "window", "detail", "format", "sort", "include_description", "include_links", "links_only", "include_disabled", "in_progress_only", "exclude_all_day", "exclude_cancelled"} {
		if !strings.Contains(string(schemaData), want) {
			t.Fatalf("upcoming_meetings schema missing %q: %s", want, schemaData)
		}
	}
	if strings.Contains(string(schemaData), "only_ongoing") {
		t.Fatalf("upcoming_meetings schema still exposes legacy only_ongoing: %s", schemaData)
	}

	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer feed.Close()

	addResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "add_calendar",
		Arguments: map[string]any{
			"key":  "work",
			"name": "Work",
			"url":  feed.URL,
		},
	})
	if err != nil || addResult.IsError {
		t.Fatalf("add_calendar result = %#v err = %v", addResult, err)
	}
	var addOut calendarOutput
	decodeStructured(t, addResult.StructuredContent, &addOut)

	upcomingResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "upcoming_meetings",
		Arguments: map[string]any{},
	})
	if err != nil || upcomingResult.IsError {
		t.Fatalf("upcoming_meetings result = %#v err = %v", upcomingResult, err)
	}
	var upcoming meetingsOutput
	decodeStructured(t, upcomingResult.StructuredContent, &upcoming)
	if len(upcoming.Meetings) != 1 || upcoming.Meetings[0].Name != "Planning" {
		t.Fatalf("upcoming meetings = %#v", upcoming.Meetings)
	}
	assertStructuredMeetingStatusFields(t, upcomingResult.StructuredContent, "meetings", "upcoming_meetings")

	formattedResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "today_meetings",
		Arguments: map[string]any{
			"format": "tg-markdownv2",
		},
	})
	if err != nil || formattedResult.IsError {
		t.Fatalf("formatted today_meetings result = %#v err = %v", formattedResult, err)
	}
	formattedData, err := json.Marshal(formattedResult.StructuredContent)
	if err != nil {
		t.Fatalf("Marshal formatted structured content error = %v", err)
	}
	var formattedRaw map[string]json.RawMessage
	if err := json.Unmarshal(formattedData, &formattedRaw); err != nil {
		t.Fatalf("Unmarshal formatted structured content error = %v", err)
	}
	for _, want := range []string{"meetings", "text"} {
		if _, ok := formattedRaw[want]; !ok {
			t.Fatalf("formatted structured content missing %q: %s", want, formattedData)
		}
	}
	var formatted meetingsOutput
	if err := json.Unmarshal(formattedData, &formatted); err != nil {
		t.Fatalf("Unmarshal formatted meetings output error = %v", err)
	}
	if !strings.Contains(formatted.Text, `*Mon Jun 29*`) || !strings.Contains(formatted.Text, `*Planning*`) {
		t.Fatalf("formatted text = %q", formatted.Text)
	}

	groupedResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "upcoming_meetings_by_calendar",
		Arguments: map[string]any{},
	})
	if err != nil || groupedResult.IsError {
		t.Fatalf("upcoming_meetings_by_calendar result = %#v err = %v", groupedResult, err)
	}
	var grouped groupedMeetingsOutput
	decodeStructured(t, groupedResult.StructuredContent, &grouped)
	if len(grouped.Calendars) != 1 || grouped.Calendars[0].CalendarName != "Work" || len(grouped.Calendars[0].Meetings) != 1 {
		t.Fatalf("grouped meetings = %#v", grouped.Calendars)
	}
	assertStructuredGroupedMeetingStatusFields(t, groupedResult.StructuredContent, "upcoming_meetings_by_calendar")

	nextOneResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "next_meeting",
		Arguments: map[string]any{},
	})
	if err != nil || nextOneResult.IsError {
		t.Fatalf("next_meeting result = %#v err = %v", nextOneResult, err)
	}
	var nextOne meetingsOutput
	decodeStructured(t, nextOneResult.StructuredContent, &nextOne)
	if len(nextOne.Meetings) != 1 || nextOne.Meetings[0].Name != "Planning" {
		t.Fatalf("next meeting = %#v", nextOne.Meetings)
	}

	nextResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "next_meetings",
		Arguments: map[string]any{},
	})
	if err != nil || nextResult.IsError {
		t.Fatalf("next_meetings result = %#v err = %v", nextResult, err)
	}
	var next meetingsOutput
	decodeStructured(t, nextResult.StructuredContent, &next)
	if len(next.Meetings) != 1 || next.Meetings[0].AllDay || next.Meetings[0].Cancelled {
		t.Fatalf("next meetings = %#v", next.Meetings)
	}
	assertStructuredMeetingStatusFields(t, nextResult.StructuredContent, "meetings", "next_meetings")

	todayResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "today_meetings",
		Arguments: map[string]any{},
	})
	if err != nil || todayResult.IsError {
		t.Fatalf("today_meetings result = %#v err = %v", todayResult, err)
	}
	var today meetingsOutput
	decodeStructured(t, todayResult.StructuredContent, &today)
	if len(today.Meetings) != 1 || today.Meetings[0].Name != "Planning" {
		t.Fatalf("today meetings = %#v", today.Meetings)
	}
	assertStructuredMeetingStatusFields(t, todayResult.StructuredContent, "meetings", "today_meetings")

	currentResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "current_meetings",
		Arguments: map[string]any{},
	})
	if err != nil || currentResult.IsError {
		t.Fatalf("current_meetings result = %#v err = %v", currentResult, err)
	}
	var current meetingsOutput
	decodeStructured(t, currentResult.StructuredContent, &current)
	if len(current.Meetings) != 1 || !current.Meetings[0].Ongoing {
		t.Fatalf("current meetings = %#v", current.Meetings)
	}

	freeBusyResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "free_busy",
		Arguments: map[string]any{
			"format": "tg-html",
		},
	})
	if err != nil || freeBusyResult.IsError {
		t.Fatalf("free_busy result = %#v err = %v", freeBusyResult, err)
	}
	freeBusyData, err := json.Marshal(freeBusyResult.StructuredContent)
	if err != nil {
		t.Fatalf("Marshal free_busy structured content error = %v", err)
	}
	var freeBusyRaw map[string]json.RawMessage
	if err := json.Unmarshal(freeBusyData, &freeBusyRaw); err != nil {
		t.Fatalf("Unmarshal free_busy structured content error = %v", err)
	}
	for _, want := range []string{"busy", "text"} {
		if _, ok := freeBusyRaw[want]; !ok {
			t.Fatalf("formatted free_busy structured content missing %q: %s", want, freeBusyData)
		}
	}
	var freeBusy freeBusyOutput
	decodeStructured(t, freeBusyResult.StructuredContent, &freeBusy)
	if len(freeBusy.Busy) != 1 || freeBusy.Busy[0].Calendar != "Work" || !strings.Contains(freeBusy.Text, "<b>Busy</b>") {
		t.Fatalf("free busy = %#v", freeBusy.Busy)
	}

	searchResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search_meetings",
		Arguments: map[string]any{"query": "plan"},
	})
	if err != nil || searchResult.IsError {
		t.Fatalf("search_meetings result = %#v err = %v", searchResult, err)
	}
	var search meetingsOutput
	decodeStructured(t, searchResult.StructuredContent, &search)
	if len(search.Meetings) != 1 || search.Meetings[0].Name != "Planning" {
		t.Fatalf("search meetings = %#v", search.Meetings)
	}
	assertStructuredMeetingStatusFields(t, searchResult.StructuredContent, "meetings", "search_meetings")

	statusResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "server_status",
		Arguments: map[string]any{},
	})
	if err != nil || statusResult.IsError {
		t.Fatalf("server_status result = %#v err = %v", statusResult, err)
	}
	var statusOut statusOutput
	decodeStructured(t, statusResult.StructuredContent, &statusOut)
	if len(statusOut.Status.Calendars) != 1 {
		t.Fatalf("server status = %#v", statusOut.Status)
	}

	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "list_calendars",
		Arguments: map[string]any{},
	})
	if err != nil || listResult.IsError {
		t.Fatalf("list_calendars result = %#v err = %v", listResult, err)
	}
	var listed calendarsOutput
	decodeStructured(t, listResult.StructuredContent, &listed)
	if len(listed.Calendars) != 1 {
		t.Fatalf("list calendars = %#v", listed.Calendars)
	}

	refreshResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "refresh_calendar",
		Arguments: map[string]any{"id": addOut.Calendar.ID},
	})
	if err != nil || refreshResult.IsError {
		t.Fatalf("refresh_calendar result = %#v err = %v", refreshResult, err)
	}
	var refreshed okOutput
	decodeStructured(t, refreshResult.StructuredContent, &refreshed)
	if !refreshed.OK {
		t.Fatalf("refresh calendar = %#v", refreshed)
	}

	refreshAllResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "refresh_all_calendars",
		Arguments: map[string]any{},
	})
	if err != nil || refreshAllResult.IsError {
		t.Fatalf("refresh_all_calendars result = %#v err = %v", refreshAllResult, err)
	}
	var refreshAll refreshAllOutput
	decodeStructured(t, refreshAllResult.StructuredContent, &refreshAll)
	if len(refreshAll.Results) != 1 || !refreshAll.Results[0].OK {
		t.Fatalf("refresh all = %#v", refreshAll)
	}

	validateFeed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sampleOneTimeICS()))
	}))
	defer validateFeed.Close()
	validateResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "validate_calendar",
		Arguments: map[string]any{"url": validateFeed.URL, "limit": 1},
	})
	if err != nil || validateResult.IsError {
		t.Fatalf("validate_calendar result = %#v err = %v", validateResult, err)
	}
	var validation ValidateCalendarResult
	decodeStructured(t, validateResult.StructuredContent, &validation)
	if !validation.OK || validation.EventCount != 1 || len(validation.Meetings) != 1 {
		t.Fatalf("validate_calendar output = %#v", validation)
	}

	updateResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "update_calendar",
		Arguments: map[string]any{"id": addOut.Calendar.ID, "name": "Renamed"},
	})
	if err != nil || updateResult.IsError {
		t.Fatalf("update_calendar result = %#v err = %v", updateResult, err)
	}

	removeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "remove_calendar",
		Arguments: map[string]any{"id": addOut.Calendar.ID},
	})
	if err != nil || removeResult.IsError {
		t.Fatalf("remove_calendar result = %#v err = %v", removeResult, err)
	}
	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 0 {
		t.Fatalf("calendars after remove = %#v", calendars)
	}
}

func containsTool(values []ToolInfo, want string) bool {
	for _, value := range values {
		if value.Name == want {
			return true
		}
	}
	return false
}

func meetingNames(meetings []Meeting) []string {
	names := make([]string, 0, len(meetings))
	for _, meeting := range meetings {
		names = append(names, meeting.Name)
	}
	return names
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewService(store, ServiceOptions{RefreshInterval: 5 * time.Minute, Lookahead: 30 * 24 * time.Hour, Timezone: "UTC"})
}

func doJSON(t *testing.T, method string, url string, in any, out any) {
	t.Helper()
	var body *bytes.Reader
	if in == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		t.Fatalf("%s %s status = %d", method, url, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
	}
}

func doText(t *testing.T, method string, url string, in any, accept string) (string, string) {
	t.Helper()
	var body *bytes.Reader
	if in == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s error = %v", method, url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		t.Fatalf("%s %s status=%d body=%s", method, url, resp.StatusCode, data)
	}
	return string(data), resp.Header.Get("Content-Type")
}

func assertOrder(t *testing.T, value string, parts ...string) {
	t.Helper()
	offset := 0
	for _, part := range parts {
		index := strings.Index(value[offset:], part)
		if index < 0 {
			t.Fatalf("%q not found after offset %d", part, offset)
		}
		offset += index + len(part)
	}
}

func decodeStructured(t *testing.T, in any, out any) {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal structured content error = %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("Unmarshal structured content error = %v", err)
	}
}

func assertStructuredMeetingStatusFields(t *testing.T, in any, key string, tool string) {
	t.Helper()
	var structured map[string][]map[string]any
	decodeStructured(t, in, &structured)
	meetings := structured[key]
	if len(meetings) != 1 {
		t.Fatalf("%s structured %s length = %d, want 1: %#v", tool, key, len(meetings), structured)
	}
	assertMeetingStatusFields(t, meetings[0], tool)
}

func assertStructuredGroupedMeetingStatusFields(t *testing.T, in any, tool string) {
	t.Helper()
	var structured struct {
		Calendars []struct {
			Meetings []map[string]any `json:"meetings"`
		} `json:"calendars"`
	}
	decodeStructured(t, in, &structured)
	if len(structured.Calendars) != 1 || len(structured.Calendars[0].Meetings) != 1 {
		t.Fatalf("%s structured grouped meetings = %#v", tool, structured.Calendars)
	}
	assertMeetingStatusFields(t, structured.Calendars[0].Meetings[0], tool)
}

func assertMeetingStatusFields(t *testing.T, meeting map[string]any, tool string) {
	t.Helper()
	for _, field := range []string{"ongoing", "all_day", "cancelled", "recurring"} {
		value, ok := meeting[field]
		if !ok {
			t.Fatalf("%s compact meeting missing false %q: %#v", tool, field, meeting)
		}
		if _, ok := value.(bool); !ok {
			t.Fatalf("%s compact meeting %q = %#v, want boolean: %#v", tool, field, value, meeting)
		}
	}
}

func rawJSON(t *testing.T, in any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal raw JSON error = %v", err)
	}
	return data
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sampleOneTimeICS() string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:planning-1\r\n" +
		"DTSTAMP:20260629T120000Z\r\n" +
		"DTSTART:20260629T130000Z\r\n" +
		"DTEND:20260629T140000Z\r\n" +
		"SUMMARY:Planning\r\n" +
		"DESCRIPTION:Roadmap\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func sampleRecurringICS() string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:daily-1\r\n" +
		"DTSTAMP:20260629T120000Z\r\n" +
		"DTSTART:20260630T150000Z\r\n" +
		"DTEND:20260630T153000Z\r\n" +
		"RRULE:FREQ=DAILY;COUNT=3\r\n" +
		"SUMMARY:Daily Standup\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func sampleExchangeWindowsTimezoneICS() string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:Microsoft Exchange Server 2010\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:exchange-1\r\n" +
		"DTSTAMP:20260629T120000Z\r\n" +
		"DTSTART;TZID=Eastern Standard Time:20260630T090000\r\n" +
		"DTEND;TZID=Eastern Standard Time:20260630T093000\r\n" +
		"SUMMARY:Exchange Meeting\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func sampleTimezoneICS(start string, end string) string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:timezone-1\r\n" +
		"DTSTAMP:20260629T120000Z\r\n" +
		icsDateLine("DTSTART", start) +
		icsDateLine("DTEND", end) +
		"SUMMARY:Timezone Meeting\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func icsDateLine(name string, value string) string {
	if strings.Contains(value, "=") {
		return name + ";" + value + "\r\n"
	}
	return name + ":" + value + "\r\n"
}

func sampleTeamsICS() string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:teams-1\r\n" +
		"DTSTAMP:20260629T120000Z\r\n" +
		"DTSTART:20260629T130000Z\r\n" +
		"DTEND:20260629T140000Z\r\n" +
		"SUMMARY:Teams Planning\r\n" +
		"DESCRIPTION:Join: https://teams.microsoft.com/l/meetup-join/abc123\\nOther: https://example.invalid/noise\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func sampleCancelledAllDayICS() string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:cancelled-all-day-1\r\n" +
		"DTSTAMP:20260629T120000Z\r\n" +
		"DTSTART:20260630T000000Z\r\n" +
		"DTEND:20260701T000000Z\r\n" +
		"SUMMARY:Canceled: Focus Day\r\n" +
		"STATUS:CANCELLED\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func sampleUntitledICS() string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:untitled-1\r\n" +
		"DTSTAMP:20260629T120000Z\r\n" +
		"DTSTART:20260629T130000Z\r\n" +
		"DTEND:20260629T140000Z\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func sampleMissingUIDICS() string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"BEGIN:VEVENT\r\n" +
		"DTSTAMP:20260629T120000Z\r\n" +
		"DTSTART:20260629T130000Z\r\n" +
		"DTEND:20260629T140000Z\r\n" +
		"SUMMARY:Missing UID\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}
