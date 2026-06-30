package icsmcp

import (
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed web/*
var webFiles embed.FS

// NewHTTPHandler builds the combined admin/API/MCP HTTP handler.
func NewHTTPHandler(svc *Service, mcpServer *mcp.Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{JSONResponse: true}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, map[string]any{"ok": true}, nil)
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		if _, err := svc.Status(r.Context()); err != nil {
			writeJSON(w, nil, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true}, nil)
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		metrics, err := svc.MetricsText(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(metrics))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		status, err := svc.Status(r.Context())
		writeJSON(w, status, err)
	})
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, openAPISpec(), nil)
	})
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>ICS MCP REST docs</title></head><body><h1>REST API</h1><p>Use the REST tab in the admin UI for an interactive tester.</p><p><a href="/">Open admin UI</a> · <a href="/openapi.json">OpenAPI JSON</a></p></body></html>`))
	})
	mux.HandleFunc("/api/rest/", func(w http.ResponseWriter, r *http.Request) {
		handleRESTTool(w, r, svc)
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		handleEventAlias(w, r, svc)
	})
	for _, suffix := range []string{".json", ".html", ".md", ".txt", ".ascii"} {
		mux.HandleFunc("/api/events"+suffix, func(w http.ResponseWriter, r *http.Request) {
			handleEventAlias(w, r, svc)
		})
	}
	mux.HandleFunc("/api/events/", func(w http.ResponseWriter, r *http.Request) {
		handleEventAlias(w, r, svc)
	})
	mux.HandleFunc("/api/free-busy", func(w http.ResponseWriter, r *http.Request) {
		handleFreeBusyAlias(w, r, svc)
	})
	for _, suffix := range []string{".json", ".html", ".md", ".txt", ".ascii"} {
		mux.HandleFunc("/api/free-busy"+suffix, func(w http.ResponseWriter, r *http.Request) {
			handleFreeBusyAlias(w, r, svc)
		})
	}
	mux.HandleFunc("/api/meetings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		query, err := upcomingQueryFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		meetings, err := svc.UpcomingMeetings(r.Context(), query)
		writeJSON(w, meetings, err)
	})
	mux.HandleFunc("/api/meetings/by-calendar", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		query, err := upcomingQueryFromRequest(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		groups, err := svc.UpcomingMeetingsByCalendar(r.Context(), query)
		writeJSON(w, groups, err)
	})
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, ToolInfos(), nil)
	})
	mux.HandleFunc("/api/tools/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/tools/")
		name, action, _ := strings.Cut(path, "/")
		if name == "" || action != "call" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in ToolCallRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := PreviewToolCall(r.Context(), svc, name, in.Arguments)
		writeJSON(w, result, err)
	})
	mux.HandleFunc("/api/calendars", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			statuses, err := svc.ListCalendarStatus(r.Context())
			writeJSON(w, statuses, err)
		case http.MethodPost:
			var in AddCalendarInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			cal, err := svc.AddCalendarAndRefresh(r.Context(), in)
			writeJSON(w, cal, err)
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/api/calendars/general-query-selection", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			selection, err := svc.GeneralQueryCalendars(r.Context())
			writeJSON(w, selection, err)
		case http.MethodPut:
			var in CalendarSelection
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			selection, err := svc.SetGeneralQueryCalendars(r.Context(), in.CalendarIDs)
			writeJSON(w, selection, err)
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/api/calendars/validate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in ValidateCalendarInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := svc.ValidateCalendar(r.Context(), in)
		writeJSON(w, result, err)
	})
	mux.HandleFunc("/api/calendars/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/calendars/")
		id, action, _ := strings.Cut(path, "/")
		if id == "" {
			http.NotFound(w, r)
			return
		}
		actionPath, _ := splitFormat(action)
		if actionPath == "events" || actionPath == "today" {
			handleCalendarEventAlias(w, r, svc, id, action)
			return
		}
		if action == "refresh" {
			if r.Method != http.MethodPost {
				methodNotAllowed(w)
				return
			}
			writeJSON(w, map[string]bool{"ok": true}, svc.RefreshCalendar(r.Context(), id, svc.now()))
			return
		}
		switch r.Method {
		case http.MethodPatch:
			var in UpdateCalendarInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			cal, err := svc.UpdateCalendar(r.Context(), id, in)
			writeJSON(w, cal, err)
		case http.MethodDelete:
			writeJSON(w, map[string]bool{"ok": true}, svc.RemoveCalendar(r.Context(), id))
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, webFiles, "web/index.html")
	})
	return mux
}

func upcomingQueryFromRequest(r *http.Request) (UpcomingQuery, error) {
	values := r.URL.Query()
	query := UpcomingQuery{}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return UpcomingQuery{}, err
		}
		query.Limit = limit
	}
	if raw := values.Get("lookahead_days"); raw != "" {
		lookahead, err := strconv.Atoi(raw)
		if err != nil {
			return UpcomingQuery{}, err
		}
		query.LookaheadDays = lookahead
	}
	query.CalendarIDs = values["calendar_id"]
	query.CalendarIDs = append(query.CalendarIDs, values["calendar"]...)
	query.Query = values.Get("query")
	query.Window = values.Get("window")
	if query.Window == "" {
		query.Window = values.Get("range")
	}
	if query.Window == "" {
		query.Window = values.Get("day")
	}
	query.Timezone = values.Get("timezone")
	query.Detail = values.Get("detail")
	query.Sort = values.Get("sort")
	query.InProgressOnly = parseBoolQuery(values.Get("in_progress_only")) || parseBoolQuery(values.Get("only_ongoing"))
	query.ExcludeAllDay = parseBoolQuery(values.Get("exclude_all_day"))
	query.ExcludeCancelled = parseBoolQuery(values.Get("exclude_cancelled"))
	query.IncludeDescription = parseBoolQuery(values.Get("include_description"))
	if raw := values.Get("include_links"); raw != "" {
		includeLinks := parseBoolQuery(raw)
		query.IncludeLinks = &includeLinks
	}
	query.LinksOnly = parseBoolQuery(values.Get("links_only"))
	query.IncludeDisabled = parseBoolQuery(values.Get("include_disabled"))
	if raw := values.Get("description_max_chars"); raw != "" {
		maxChars, err := strconv.Atoi(raw)
		if err != nil {
			return UpcomingQuery{}, err
		}
		query.DescriptionMaxChars = maxChars
	}
	if raw := values.Get("after"); raw != "" {
		after, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return UpcomingQuery{}, err
		}
		query.After = after
	}
	if raw := values.Get("before"); raw != "" {
		before, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return UpcomingQuery{}, err
		}
		query.Before = before
	}
	return query, nil
}

func handleRESTTool(w http.ResponseWriter, r *http.Request, svc *Service) {
	toolName, format := splitFormat(strings.TrimPrefix(r.URL.Path, "/api/rest/"))
	if toolName == "" || strings.Contains(toolName, "/") {
		http.NotFound(w, r)
		return
	}
	info, ok := toolInfoByName(toolName)
	if !ok {
		http.NotFound(w, r)
		return
	}
	var raw json.RawMessage
	switch r.Method {
	case http.MethodGet:
		if info.Category != "read" || !info.ReadOnly {
			methodNotAllowed(w)
			return
		}
		arguments, err := restReadArguments(r, toolName)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		raw = arguments
	case http.MethodPost:
		if info.Category != "admin" {
			methodNotAllowed(w)
			return
		}
		arguments, err := readRESTPostArguments(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		raw = arguments
	default:
		methodNotAllowed(w)
		return
	}
	result, err := PreviewToolCall(r.Context(), svc, toolName, raw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeFormatted(w, r, result.Result, format)
}

func handleEventAlias(w http.ResponseWriter, r *http.Request, svc *Service) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	path, format := splitFormat(strings.TrimPrefix(r.URL.Path, "/api/events"))
	query, err := upcomingQueryFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	var value any
	switch strings.Trim(path, "/") {
	case "":
		value, err = svc.UpcomingMeetings(r.Context(), query)
	case "by-calendar":
		value, err = svc.UpcomingMeetingsByCalendar(r.Context(), query)
	case "today":
		value, err = svc.TodayMeetings(r.Context(), query)
	case "tomorrow":
		query.Window = "tomorrow"
		query.OverlapWindow = true
		value, err = svc.UpcomingMeetings(r.Context(), query)
	case "today-tomorrow", "today_tomorrow":
		query.Window = "today_tomorrow"
		query.OverlapWindow = true
		value, err = svc.UpcomingMeetings(r.Context(), query)
	case "current":
		query.InProgressOnly = true
		value, err = svc.UpcomingMeetings(r.Context(), query)
	case "next":
		query.ExcludeAllDay = true
		query.ExcludeCancelled = true
		value, err = svc.UpcomingMeetings(r.Context(), query)
	case "search":
		value, err = svc.UpcomingMeetings(r.Context(), query)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeFormatted(w, r, value, format)
}

func handleFreeBusyAlias(w http.ResponseWriter, r *http.Request, svc *Service) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	_, format := splitFormat(strings.TrimPrefix(r.URL.Path, "/api/free-busy"))
	query, err := upcomingQueryFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	busy, err := svc.FreeBusy(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeFormatted(w, r, busy, format)
}

func handleCalendarEventAlias(w http.ResponseWriter, r *http.Request, svc *Service, calendar string, action string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	query, err := upcomingQueryFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	calendarID, err := resolveCalendarID(r, svc, calendar)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	query.CalendarIDs = append(query.CalendarIDs, calendarID)
	actionPath, format := splitFormat(action)
	if actionPath == "today" {
		meetings, err := svc.TodayMeetings(r.Context(), query)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeFormatted(w, r, meetings, format)
		return
	}
	meetings, err := svc.UpcomingMeetings(r.Context(), query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeFormatted(w, r, meetings, format)
}

func restReadArguments(r *http.Request, toolName string) (json.RawMessage, error) {
	switch toolName {
	case "upcoming_meetings", "upcoming_meetings_by_calendar", "next_meeting", "next_meetings", "today_meetings", "current_meetings", "search_meetings", "free_busy":
		query, err := upcomingQueryFromRequest(r)
		if err != nil {
			return nil, err
		}
		return json.Marshal(query)
	default:
		return nil, nil
	}
}

func readRESTPostArguments(r *http.Request) (json.RawMessage, error) {
	var raw json.RawMessage
	if r.Body == nil {
		return nil, nil
	}
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var wrapped ToolCallRequest
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Arguments) > 0 {
		return wrapped.Arguments, nil
	}
	return raw, nil
}

func toolInfoByName(name string) (ToolInfo, bool) {
	for _, info := range ToolInfos() {
		if info.Name == name {
			return info, true
		}
	}
	return ToolInfo{}, false
}

func resolveCalendarID(r *http.Request, svc *Service, value string) (string, error) {
	calendars, err := svc.ListCalendars(r.Context())
	if err != nil {
		return "", err
	}
	for _, calendar := range calendars {
		if calendar.ID == value || strings.EqualFold(calendar.Key, value) {
			return calendar.ID, nil
		}
	}
	return "", fmt.Errorf("calendar %q not found", value)
}

func splitFormat(path string) (string, string) {
	for _, format := range []string{"json", "html", "md", "txt", "ascii"} {
		suffix := "." + format
		if strings.HasSuffix(path, suffix) {
			return strings.TrimSuffix(path, suffix), format
		}
	}
	return path, ""
}

func negotiatedFormat(r *http.Request, pathFormat string) string {
	if format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format"))); format != "" {
		return format
	}
	if pathFormat != "" {
		return pathFormat
	}
	accept := r.Header.Get("Accept")
	switch {
	case strings.Contains(accept, "text/html"):
		return "html"
	case strings.Contains(accept, "text/markdown"):
		return "md"
	case strings.Contains(accept, "text/plain"):
		return "txt"
	default:
		return "json"
	}
}

func writeFormatted(w http.ResponseWriter, r *http.Request, value any, pathFormat string) {
	switch negotiatedFormat(r, pathFormat) {
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderHTML(value)))
	case "md":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(renderMarkdown(value)))
	case "txt", "ascii":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(renderText(value)))
	default:
		writeJSON(w, value, nil)
	}
}

func renderHTML(value any) string {
	return "<!doctype html><html><body><pre>" + html.EscapeString(renderText(value)) + "</pre></body></html>"
}

func renderMarkdown(value any) string {
	return "# " + renderTitle(value) + "\n\n" + renderText(value)
}

func renderText(value any) string {
	var b strings.Builder
	switch typed := value.(type) {
	case meetingsOutput:
		writeMeetingsText(&b, typed.Meetings)
	case groupedMeetingsOutput:
		writeGroupsText(&b, typed.Calendars)
	case freeBusyOutput:
		writeBusyText(&b, typed.Busy)
	case []Meeting:
		writeMeetingsText(&b, typed)
	case []CalendarMeetingGroup:
		writeGroupsText(&b, typed)
	case []BusyBlock:
		writeBusyText(&b, typed)
	default:
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data) + "\n"
	}
	return b.String()
}

func renderTitle(value any) string {
	switch value.(type) {
	case groupedMeetingsOutput, []CalendarMeetingGroup:
		return "Meetings By Calendar"
	case freeBusyOutput, []BusyBlock:
		return "Free Busy"
	default:
		return "Meetings"
	}
}

func writeMeetingsText(b *strings.Builder, meetings []Meeting) {
	if len(meetings) == 0 {
		b.WriteString("No meetings.\n")
		return
	}
	for _, meeting := range meetings {
		_, _ = fmt.Fprintf(b, "- %s | %s | %s | %s\n", meeting.When, meeting.CalendarName, meeting.Name, meeting.Duration)
	}
}

func writeGroupsText(b *strings.Builder, groups []CalendarMeetingGroup) {
	if len(groups) == 0 {
		b.WriteString("No meetings.\n")
		return
	}
	for _, group := range groups {
		_, _ = fmt.Fprintf(b, "%s\n", group.CalendarName)
		writeMeetingsText(b, group.Meetings)
	}
}

func writeBusyText(b *strings.Builder, busy []BusyBlock) {
	if len(busy) == 0 {
		b.WriteString("No busy blocks.\n")
		return
	}
	for _, block := range busy {
		_, _ = fmt.Fprintf(b, "- %s | %s | %s\n", block.When, block.Calendar, block.Duration)
	}
}

func openAPISpec() map[string]any {
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":   "ICS MCP REST API",
			"version": "2.0.0",
		},
		"paths": map[string]any{
			"/api/rest/{tool_name}":            map[string]any{"get": map[string]any{"summary": "Call a read-only MCP tool"}, "post": map[string]any{"summary": "Call an admin MCP tool"}},
			"/api/events":                      map[string]any{"get": map[string]any{"summary": "Upcoming events"}},
			"/api/events/by-calendar":          map[string]any{"get": map[string]any{"summary": "Upcoming events grouped by calendar"}},
			"/api/events/today":                map[string]any{"get": map[string]any{"summary": "Today's events"}},
			"/api/events/tomorrow":             map[string]any{"get": map[string]any{"summary": "Tomorrow's events"}},
			"/api/events/today-tomorrow":       map[string]any{"get": map[string]any{"summary": "Today and tomorrow events"}},
			"/api/events/current":              map[string]any{"get": map[string]any{"summary": "Current events"}},
			"/api/events/next":                 map[string]any{"get": map[string]any{"summary": "Next meeting-focused events"}},
			"/api/events/search":               map[string]any{"get": map[string]any{"summary": "Search events"}},
			"/api/free-busy":                   map[string]any{"get": map[string]any{"summary": "Free/busy blocks"}},
			"/api/calendars/{calendar}/events": map[string]any{"get": map[string]any{"summary": "Events for one calendar"}},
			"/api/calendars/{calendar}/today":  map[string]any{"get": map[string]any{"summary": "Today's events for one calendar"}},
		},
	}
}

func parseBoolQuery(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
}
