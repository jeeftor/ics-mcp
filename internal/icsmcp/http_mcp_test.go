package icsmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestHTTPAPIManagesCalendarsAndServesAdminUI(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/")
	if err != nil {
		t.Fatalf("GET / error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	var add Calendar
	doJSON(t, http.MethodPost, server.URL+"/api/calendars", AddCalendarInput{
		Key:  "team",
		Name: "Team",
		URL:  "https://example.test/team.ics",
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

	var status Status
	doJSON(t, http.MethodGet, server.URL+"/api/status", nil, &status)
	if len(status.Calendars) != 1 || status.Calendars[0].Name != "Renamed" {
		t.Fatalf("status = %#v", status)
	}

	doJSON(t, http.MethodDelete, server.URL+"/api/calendars/"+add.ID, nil, nil)
	doJSON(t, http.MethodGet, server.URL+"/api/calendars", nil, &list)
	if len(list) != 0 {
		t.Fatalf("calendar list after delete = %#v", list)
	}

	_, _ = ctx, svc
}

func TestMCPToolsExposeMeetingsAndAdminMutations(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
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
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		toolNames = append(toolNames, tool.Name)
	}
	for _, want := range []string{"upcoming_meetings", "calendar_list", "calendar_add", "calendar_update", "calendar_remove", "calendar_refresh"} {
		if !contains(toolNames, want) {
			t.Fatalf("tool names = %#v, missing %s", toolNames, want)
		}
	}

	addResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "calendar_add",
		Arguments: map[string]any{
			"key":  "work",
			"name": "Work",
			"url":  "https://example.test/work.ics",
		},
	})
	if err != nil || addResult.IsError {
		t.Fatalf("calendar_add result = %#v err = %v", addResult, err)
	}
	var addOut calendarOutput
	decodeStructured(t, addResult.StructuredContent, &addOut)

	if err := svc.ReplaceEvents(ctx, addOut.Calendar.ID, []EventInstance{{
		CalendarID:   addOut.Calendar.ID,
		CalendarName: "Work",
		Name:         "Planning",
		Start:        now.Add(1 * time.Hour),
		End:          now.Add(90 * time.Minute),
	}}); err != nil {
		t.Fatalf("ReplaceEvents() error = %v", err)
	}

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

	updateResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "calendar_update",
		Arguments: map[string]any{"id": addOut.Calendar.ID, "name": "Renamed"},
	})
	if err != nil || updateResult.IsError {
		t.Fatalf("calendar_update result = %#v err = %v", updateResult, err)
	}

	removeResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "calendar_remove",
		Arguments: map[string]any{"id": addOut.Calendar.ID},
	})
	if err != nil || removeResult.IsError {
		t.Fatalf("calendar_remove result = %#v err = %v", removeResult, err)
	}
	calendars, err := svc.ListCalendars(ctx)
	if err != nil {
		t.Fatalf("ListCalendars() error = %v", err)
	}
	if len(calendars) != 0 {
		t.Fatalf("calendars after remove = %#v", calendars)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return NewService(store, ServiceOptions{RefreshInterval: 5 * time.Minute, Lookahead: 30 * 24 * time.Hour})
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
