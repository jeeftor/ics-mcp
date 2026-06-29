package icsmcp

import (
	"embed"
	"encoding/json"
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
	query.Query = values.Get("query")
	query.OnlyOngoing = parseBoolQuery(values.Get("only_ongoing"))
	query.ExcludeAllDay = parseBoolQuery(values.Get("exclude_all_day"))
	query.ExcludeCancelled = parseBoolQuery(values.Get("exclude_cancelled"))
	query.IncludeDescription = parseBoolQuery(values.Get("include_description"))
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
