package icsmcp

import (
	"bytes"
	"database/sql"
	"embed"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errUnauthorized = errors.New("authentication required")

//go:embed web/dist/*
var webFiles embed.FS

// NewHTTPHandler builds the combined admin/API/MCP HTTP handler.
func NewHTTPHandler(svc *Service, mcpServer *mcp.Server) http.Handler {
	return NewHTTPHandlerWithOptions(svc, mcpServer, HTTPOptions{})
}

// NewHTTPHandlerWithOptions builds the combined admin/API/MCP handler with an optional bearer-token boundary.
func NewHTTPHandlerWithOptions(svc *Service, mcpServer *mcp.Server, options HTTPOptions) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return mcpServer
	}, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true, PropagateRequestCancellation: true}))
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
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, svc.RuntimeConfig(), nil)
		case http.MethodPut:
			var in UpdateRuntimeConfigInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			config, err := svc.UpdateRuntimeConfig(r.Context(), in)
			writeJSON(w, config, err)
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/api/environment", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		variables, err := svc.EnvironmentVariables(r.Context())
		writeJSON(w, variables, err)
	})
	mux.HandleFunc("/api/update-check", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		update, err := svc.UpdateCheck(r.Context())
		writeJSON(w, update, err)
	})
	mux.HandleFunc("/api/llm-profile", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			profile, err := svc.LLMProfile(r.Context())
			writeJSON(w, profile, err)
		case http.MethodPut:
			var in UpdateLLMProfileInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			profile, err := svc.UpdateLLMProfile(r.Context(), in)
			writeJSON(w, profile, err)
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/api/llm-profile/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		writeJSON(w, map[string]bool{"ok": true}, svc.TestLLMProfile(r.Context()))
	})
	mux.HandleFunc("/api/llm-profile/endpoint-test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in LLMConnectionInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "Endpoint reached."}, svc.TestLLMEndpoint(r.Context(), in))
	})
	mux.HandleFunc("/api/llm-profile/models", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in LLMConnectionInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		models, err := svc.DiscoverLLMModels(r.Context(), in)
		writeJSON(w, map[string]any{"models": models}, err)
	})
	mux.HandleFunc("/api/llm-profile/model-test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in LLMModelTestInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "message": "Model responded."}, svc.TestLLMModel(r.Context(), in))
	})
	mux.HandleFunc("/api/llm-profile/lemonade/model-status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in LLMModelTestInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := svc.LemonadeModelStatus(r.Context(), in)
		writeJSON(w, result, err)
	})
	mux.HandleFunc("/api/llm-profile/lemonade/model-load", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in LLMModelTestInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		result, err := svc.LoadLemonadeModel(r.Context(), in)
		writeJSON(w, result, err)
	})
	mux.HandleFunc("/api/insight-inquiries", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			inquiries, err := svc.ListInsightInquiries(r.Context())
			writeJSON(w, inquiries, err)
		case http.MethodPost:
			var body struct {
				Name string `json:"name"`
				SaveInsightInquiryInput
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			inquiry, err := svc.SaveInsightInquiry(r.Context(), body.Name, body.SaveInsightInquiryInput)
			writeJSON(w, inquiry, err)
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/api/insight-inquiries/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/insight-inquiries/")
		switch r.Method {
		case http.MethodGet:
			inquiry, err := svc.GetInsightInquiry(r.Context(), name)
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, inquiry, err)
		case http.MethodPut:
			var in SaveInsightInquiryInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			inquiry, err := svc.SaveInsightInquiry(r.Context(), name, in)
			writeJSON(w, inquiry, err)
		case http.MethodDelete:
			err := svc.DeleteInsightInquiry(r.Context(), name)
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, map[string]bool{"ok": true}, err)
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/api/insights", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			insights, err := svc.ListInsights(r.Context())
			writeJSON(w, insights, err)
		case http.MethodPost:
			var in RunInsightInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			insight, err := svc.RunInsight(r.Context(), in)
			writeJSON(w, insight, err)
		default:
			methodNotAllowed(w)
		}
	})
	mux.HandleFunc("/api/insights/preview", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			methodNotAllowed(w)
			return
		}
		var in RunInsightInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		insight, err := svc.PreviewInsight(r.Context(), in)
		writeJSON(w, insight, err)
	})
	mux.HandleFunc("/api/insights/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		insight, err := svc.GetInsight(r.Context(), strings.TrimPrefix(r.URL.Path, "/api/insights/"))
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, insight, err)
	})
	// Versioned prompt routes expose saved inquiry definitions and their cached
	// LLM outputs. They are intentionally read-only: no GET route invokes a
	// provider or changes prompt state.
	mux.HandleFunc("/api/v1/prompts", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		outputs, err := svc.ListPromptOutputs(r.Context())
		writeJSON(w, outputs, err)
	})
	mux.HandleFunc("/api/v1/prompts/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/prompts/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] == "" || len(parts) > 2 {
			http.NotFound(w, r)
			return
		}
		name := parts[0]
		if len(parts) == 1 {
			inquiry, err := svc.GetInsightInquiry(r.Context(), name)
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, inquiry, err)
			return
		}
		switch parts[1] {
		case "output":
			output, err := svc.GetPromptOutput(r.Context(), name)
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, output, err)
		case "history":
			limit := 10
			if raw := r.URL.Query().Get("limit"); raw != "" {
				var err error
				limit, err = strconv.Atoi(raw)
				if err != nil || limit < 1 {
					writeError(w, http.StatusBadRequest, fmt.Errorf("limit must be a positive integer"))
					return
				}
			}
			history, err := svc.ListPromptHistory(r.Context(), name, limit)
			if errors.Is(err, sql.ErrNoRows) {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, history, err)
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		status, _ := svc.Status(r.Context())
		writeJSON(w, openAPISpec(status.Version), nil)
	})
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		status, _ := svc.Status(r.Context())
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(openAPIDocs(status.Version)))
	})
	mux.HandleFunc("/api/rest/", func(w http.ResponseWriter, r *http.Request) {
		handleRESTTool(w, r, svc)
	})
	mux.HandleFunc("/api/events", func(w http.ResponseWriter, r *http.Request) {
		handleEventAlias(w, r, svc)
	})
	for _, suffix := range []string{".json", ".html", ".md", ".txt", ".ascii", ".csv"} {
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
	for _, suffix := range []string{".json", ".html", ".md", ".txt", ".ascii", ".csv"} {
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
		if err == nil && !isJSONMeetingFormat(query.Format) {
			text, formatErr := FormatMeetings(meetings, query.Format)
			writeText(w, text, formatErr)
			return
		}
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
		if err == nil && !isJSONMeetingFormat(query.Format) {
			text, formatErr := FormatGroupedMeetings(groups, query.Format)
			writeText(w, text, formatErr)
			return
		}
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
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		tags, err := svc.ListTags(r.Context())
		writeJSON(w, tags, err)
	})
	mux.HandleFunc("/api/tags/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/tags/")
		if name == "" || strings.Contains(name, "/") {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodPut && r.Method != http.MethodPatch {
			methodNotAllowed(w)
			return
		}
		var in UpdateCalendarTagInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		tag, err := svc.UpdateTag(r.Context(), name, in)
		writeJSON(w, tag, err)
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
		if action == "custom-icon" {
			handleCalendarCustomIcon(w, r, svc, id)
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
		if r.URL.Path != "/" && !isAdminSPARoute(r.URL.Path) {
			assetPath := strings.TrimPrefix(r.URL.Path, "/")
			if (strings.HasPrefix(assetPath, "assets/") || assetPath == "icsmcp-mark.svg") && fs.ValidPath(assetPath) {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				http.ServeFileFS(w, r, webFiles, "web/dist/"+assetPath)
				return
			}
			if handleCalendarMeetingShortcut(w, r, svc) {
				return
			}
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, webFiles, "web/dist/index.html")
	})
	return authMiddleware(mux, options.BearerToken)
}

// isAdminSPARoute reserves addressable console tabs before legacy calendar
// shortcuts get a chance to interpret their first path segment as a calendar.
func isAdminSPARoute(path string) bool {
	switch path {
	case "/ai", "/config", "/config/runtime", "/config/environment", "/config/calendars", "/config/tags", "/config/llm", "/api", "/api/mcp-tools", "/api/meeting-preview", "/api/rest-explorer", "/api/openapi":
		return true
	default:
		return false
	}
}

func handleCalendarCustomIcon(w http.ResponseWriter, r *http.Request, svc *Service, id string) {
	switch r.Method {
	case http.MethodGet:
		contentType, data, err := svc.CalendarCustomIcon(r.Context(), id)
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, errors.New("calendar custom icon not found"))
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(data)
	case http.MethodPut:
		contentType, err := calendarIconContentType(r.Header.Get("Content-Type"))
		if err != nil {
			writeError(w, http.StatusUnsupportedMediaType, err)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, MaxCalendarCustomIconBytes+1)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("calendar icon must be at most %d KiB", MaxCalendarCustomIconBytes>>10))
			return
		}
		if int64(len(data)) > MaxCalendarCustomIconBytes {
			writeError(w, http.StatusRequestEntityTooLarge, fmt.Errorf("calendar icon must be at most %d KiB", MaxCalendarCustomIconBytes>>10))
			return
		}
		if err := validateCalendarCustomIcon(contentType, data); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := svc.SetCalendarCustomIcon(r.Context(), id, contentType, data); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]string{"custom_icon_url": "/api/calendars/" + id + "/custom-icon"}, nil)
	case http.MethodDelete:
		if err := svc.ClearCalendarCustomIcon(r.Context(), id); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true}, nil)
	default:
		methodNotAllowed(w)
	}
}

func calendarIconContentType(header string) (string, error) {
	contentType, _, err := mime.ParseMediaType(header)
	if err != nil {
		return "", fmt.Errorf("parse icon content type: %w", err)
	}
	if contentType != "image/svg+xml" && contentType != "image/gif" {
		return "", errors.New("calendar icon must be image/svg+xml or image/gif")
	}
	return contentType, nil
}

func validateCalendarCustomIcon(contentType string, data []byte) error {
	if len(data) == 0 {
		return errors.New("calendar icon is empty")
	}
	if contentType == "image/gif" {
		if len(data) < 6 || (!bytes.Equal(data[:6], []byte("GIF87a")) && !bytes.Equal(data[:6], []byte("GIF89a"))) {
			return errors.New("calendar icon is not a valid GIF")
		}
		return nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	rootSeen := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return errors.New("calendar icon is not valid SVG")
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		name := strings.ToLower(start.Name.Local)
		if !rootSeen {
			if name != "svg" {
				return errors.New("calendar icon SVG must have an svg root element")
			}
			rootSeen = true
		}
		if name == "script" || name == "foreignobject" || name == "iframe" || name == "object" || name == "embed" || name == "style" {
			return errors.New("calendar icon SVG contains an unsafe element")
		}
		for _, attr := range start.Attr {
			attrName := strings.ToLower(attr.Name.Local)
			value := strings.TrimSpace(strings.ToLower(attr.Value))
			if attr.Name.Space == "xmlns" || attrName == "xmlns" {
				continue
			}
			if strings.HasPrefix(attrName, "on") || attrName == "style" {
				return errors.New("calendar icon SVG contains an unsafe attribute")
			}
			if strings.HasPrefix(value, "javascript:") || strings.HasPrefix(value, "data:") || strings.HasPrefix(value, "http:") || strings.HasPrefix(value, "https:") || strings.HasPrefix(value, "//") {
				return errors.New("calendar icon SVG contains an external reference")
			}
			if (attrName == "href" || attrName == "src") && value != "" && !strings.HasPrefix(value, "#") {
				return errors.New("calendar icon SVG contains an external reference")
			}
		}
	}
	if !rootSeen {
		return errors.New("calendar icon SVG must have an svg root element")
	}
	return nil
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
	query.Tags = values["tag"]
	query.Tags = append(query.Tags, values["tags"]...)
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
	query.Format = values.Get("format")
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
	writeFormatted(w, r, freeBusyOutput{Busy: busy}, format)
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

func handleCalendarMeetingShortcut(w http.ResponseWriter, r *http.Request, svc *Service) bool {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return true
	}
	input, format, ok, err := calendarMeetingShortcutFromRequest(r)
	if !ok {
		return false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return true
	}
	meeting, err := svc.CalendarMeeting(r.Context(), input)
	if err != nil {
		writeCalendarMeetingShortcutError(w, err)
		return true
	}
	writeFormatted(w, r, meeting, format)
	return true
}

func calendarMeetingShortcutFromRequest(r *http.Request) (calendarMeetingInput, string, bool, error) {
	path, format := splitFormat(strings.Trim(r.URL.Path, "/"))
	parts := strings.Split(path, "/")
	if len(parts) != 2 && len(parts) != 3 {
		return calendarMeetingInput{}, "", false, nil
	}
	if isReservedShortcutRoot(parts[0]) {
		return calendarMeetingInput{}, "", false, nil
	}
	query, err := upcomingQueryFromRequest(r)
	if err != nil {
		return calendarMeetingInput{}, "", true, err
	}
	input := calendarMeetingInput{UpcomingQuery: query, Calendar: parts[0], List: "upcoming"}
	indexPart := parts[1]
	if len(parts) == 3 {
		input.List = strings.ToLower(strings.TrimSpace(parts[1]))
		if input.List != "upcoming" && input.List != "ongoing" {
			return calendarMeetingInput{}, "", false, nil
		}
		indexPart = parts[2]
	}
	index, err := strconv.Atoi(indexPart)
	if err != nil || index <= 0 {
		return calendarMeetingInput{}, "", true, fmt.Errorf("meeting index must be a positive integer")
	}
	input.Index = index
	return input, format, true, nil
}

func isReservedShortcutRoot(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "api", "docs", "openapi.json", "healthz", "readyz", "metrics", "mcp":
		return true
	default:
		return false
	}
}

func writeCalendarMeetingShortcutError(w http.ResponseWriter, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "meeting index must be a positive integer"), strings.Contains(message, "unknown calendar meeting list"):
		writeError(w, http.StatusBadRequest, err)
	case strings.Contains(message, "not found"), strings.Contains(message, "has no"):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}

func restReadArguments(r *http.Request, toolName string) (json.RawMessage, error) {
	switch toolName {
	case "upcoming_meetings", "upcoming_meetings_by_calendar", "next_meeting", "next_meetings", "today_meetings", "current_meetings", "search_meetings", "free_busy":
		query, err := upcomingQueryFromRequest(r)
		if err != nil {
			return nil, err
		}
		return json.Marshal(query)
	case "calendar_meeting":
		input, err := calendarMeetingInputFromRequest(r)
		if err != nil {
			return nil, err
		}
		return json.Marshal(input)
	default:
		return nil, nil
	}
}

func calendarMeetingInputFromRequest(r *http.Request) (calendarMeetingInput, error) {
	query, err := upcomingQueryFromRequest(r)
	if err != nil {
		return calendarMeetingInput{}, err
	}
	values := r.URL.Query()
	index, err := strconv.Atoi(values.Get("index"))
	if err != nil || index <= 0 {
		return calendarMeetingInput{}, fmt.Errorf("meeting index must be a positive integer")
	}
	calendar := values.Get("calendar_key")
	if calendar == "" {
		calendar = values.Get("calendar")
	}
	return calendarMeetingInput{
		UpcomingQuery: query,
		Calendar:      calendar,
		Index:         index,
		List:          values.Get("list"),
	}, nil
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
	return svc.ResolveCalendarID(r.Context(), value)
}

func splitFormat(path string) (string, string) {
	for _, format := range []string{"json", "html", "md", "txt", "ascii", "csv"} {
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
	case strings.Contains(accept, "text/csv"):
		return "csv"
	case strings.Contains(accept, "text/plain"):
		return "txt"
	default:
		return "json"
	}
}

func writeFormatted(w http.ResponseWriter, r *http.Request, value any, pathFormat string) {
	options := tableFormatOptionsFromRequest(r)
	format := negotiatedFormat(r, pathFormat)
	if formatted, ok, err := telegramFormattedText(value, format); ok {
		writeText(w, formatted, err)
		return
	}
	switch format {
	case "html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(renderHTML(value, options)))
	case "md":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(renderMarkdown(value, options)))
	case "txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(renderText(value)))
	case "ascii":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(renderASCII(value, options)))
	case "csv":
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(renderCSV(value, options)))
	default:
		writeJSON(w, value, nil)
	}
}

func telegramFormattedText(value any, format string) (string, bool, error) {
	normalized, err := NormalizeMeetingFormat(format)
	if err != nil {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(format)), "tg-") || strings.EqualFold(format, "telegram") || strings.EqualFold(format, "text") {
			return "", true, err
		}
		return "", false, nil
	}
	if normalized == MeetingFormatJSON {
		return "", false, nil
	}
	switch typed := value.(type) {
	case meetingOutput:
		text, formatErr := FormatMeetings([]Meeting{typed.Meeting}, normalized)
		return text, true, formatErr
	case meetingsOutput:
		text, formatErr := FormatMeetings(typed.Meetings, normalized)
		return text, true, formatErr
	case groupedMeetingsOutput:
		text, formatErr := FormatGroupedMeetings(typed.Calendars, normalized)
		return text, true, formatErr
	case freeBusyOutput:
		text, formatErr := FormatBusyBlocks(typed.Busy, normalized)
		return text, true, formatErr
	case []Meeting:
		text, formatErr := FormatMeetings(typed, normalized)
		return text, true, formatErr
	case []CalendarMeetingGroup:
		text, formatErr := FormatGroupedMeetings(typed, normalized)
		return text, true, formatErr
	case []BusyBlock:
		text, formatErr := FormatBusyBlocks(typed, normalized)
		return text, true, formatErr
	case Meeting:
		text, formatErr := FormatMeetings([]Meeting{typed}, normalized)
		return text, true, formatErr
	default:
		return "", false, nil
	}
}

type tableFormatOptions struct {
	fields       []string
	timeStyle    string
	showTimezone bool
}

func tableFormatOptionsFromRequest(r *http.Request) tableFormatOptions {
	timeStyle := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("time_style")))
	if timeStyle == "" {
		timeStyle = "date_range"
	}
	return tableFormatOptions{
		fields:       selectedFields(r),
		timeStyle:    timeStyle,
		showTimezone: parseBoolQuery(r.URL.Query().Get("show_timezone")),
	}
}

func selectedFields(r *http.Request) []string {
	raw := strings.TrimSpace(r.URL.Query().Get("fields"))
	if raw == "" {
		return []string{"when", "calendar", "title", "duration"}
	}
	fields := []string{}
	for _, field := range strings.Split(raw, ",") {
		field = strings.ToLower(strings.TrimSpace(field))
		if field != "" {
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return []string{"when", "calendar", "title", "duration"}
	}
	return fields
}

func renderHTML(value any, options tableFormatOptions) string {
	return "<!doctype html><html><head><style>body{font-family:system-ui,sans-serif}table{border-collapse:collapse}th,td{border:1px solid #ccc;padding:6px 8px;text-align:left}</style></head><body>" + renderHTMLBody(value, options) + "</body></html>"
}

func renderMarkdown(value any, options tableFormatOptions) string {
	return "# " + renderTitle(value) + "\n\n" + renderMarkdownBody(value, options)
}

func renderText(value any) string {
	var b strings.Builder
	switch typed := value.(type) {
	case meetingOutput:
		writeMeetingsText(&b, []Meeting{typed.Meeting})
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
	case Meeting:
		writeMeetingsText(&b, []Meeting{typed})
	default:
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data) + "\n"
	}
	return b.String()
}

func renderASCII(value any, options tableFormatOptions) string {
	switch typed := value.(type) {
	case meetingOutput:
		return renderMeetingsASCII([]Meeting{typed.Meeting}, options)
	case meetingsOutput:
		return renderMeetingsASCII(typed.Meetings, options)
	case groupedMeetingsOutput:
		return renderGroupsASCII(typed.Calendars, options)
	case freeBusyOutput:
		return renderBusyASCII(typed.Busy, options)
	case []Meeting:
		return renderMeetingsASCII(typed, options)
	case []CalendarMeetingGroup:
		return renderGroupsASCII(typed, options)
	case []BusyBlock:
		return renderBusyASCII(typed, options)
	case Meeting:
		return renderMeetingsASCII([]Meeting{typed}, options)
	default:
		return renderText(value)
	}
}

func renderCSV(value any, options tableFormatOptions) string {
	var rows [][]string
	switch typed := value.(type) {
	case meetingOutput:
		rows = meetingRows([]Meeting{typed.Meeting}, options)
	case meetingsOutput:
		rows = meetingRows(typed.Meetings, options)
	case groupedMeetingsOutput:
		rows = groupRows(typed.Calendars, options)
	case freeBusyOutput:
		rows = busyRows(typed.Busy, options)
	case []Meeting:
		rows = meetingRows(typed, options)
	case []CalendarMeetingGroup:
		rows = groupRows(typed, options)
	case []BusyBlock:
		rows = busyRows(typed, options)
	case Meeting:
		rows = meetingRows([]Meeting{typed}, options)
	default:
		return renderText(value)
	}
	var b strings.Builder
	for _, row := range rows {
		for index, cell := range row {
			if index > 0 {
				b.WriteString(",")
			}
			b.WriteString(csvCell(cell))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func renderTitle(value any) string {
	switch value.(type) {
	case meetingOutput, Meeting:
		return "Meeting"
	case groupedMeetingsOutput, []CalendarMeetingGroup:
		return "Meetings By Calendar"
	case freeBusyOutput, []BusyBlock:
		return "Free Busy"
	default:
		return "Meetings"
	}
}

func renderHTMLBody(value any, options tableFormatOptions) string {
	switch typed := value.(type) {
	case meetingOutput:
		return renderMeetingsHTML([]Meeting{typed.Meeting}, options)
	case meetingsOutput:
		return renderMeetingsHTML(typed.Meetings, options)
	case groupedMeetingsOutput:
		return renderGroupsHTML(typed.Calendars, options)
	case freeBusyOutput:
		return renderBusyHTML(typed.Busy, options)
	case []Meeting:
		return renderMeetingsHTML(typed, options)
	case []CalendarMeetingGroup:
		return renderGroupsHTML(typed, options)
	case []BusyBlock:
		return renderBusyHTML(typed, options)
	case Meeting:
		return renderMeetingsHTML([]Meeting{typed}, options)
	default:
		return "<pre>" + html.EscapeString(renderText(value)) + "</pre>"
	}
}

func renderMarkdownBody(value any, options tableFormatOptions) string {
	switch typed := value.(type) {
	case meetingOutput:
		return renderMeetingsMarkdown([]Meeting{typed.Meeting}, options)
	case meetingsOutput:
		return renderMeetingsMarkdown(typed.Meetings, options)
	case groupedMeetingsOutput:
		return renderGroupsMarkdown(typed.Calendars, options)
	case freeBusyOutput:
		return renderBusyMarkdown(typed.Busy, options)
	case []Meeting:
		return renderMeetingsMarkdown(typed, options)
	case []CalendarMeetingGroup:
		return renderGroupsMarkdown(typed, options)
	case []BusyBlock:
		return renderBusyMarkdown(typed, options)
	case Meeting:
		return renderMeetingsMarkdown([]Meeting{typed}, options)
	default:
		return "```json\n" + strings.TrimSpace(renderText(value)) + "\n```\n"
	}
}

func renderMeetingsHTML(meetings []Meeting, options tableFormatOptions) string {
	if len(meetings) == 0 {
		return "<p>No meetings.</p>"
	}
	var b strings.Builder
	writeHTMLTable(&b, fieldLabels(options.fields), meetingRows(meetings, options)[1:])
	return b.String()
}

func renderGroupsHTML(groups []CalendarMeetingGroup, options tableFormatOptions) string {
	if len(groups) == 0 {
		return "<p>No meetings.</p>"
	}
	var b strings.Builder
	for _, group := range groups {
		_, _ = fmt.Fprintf(&b, "<h2>%s</h2>", html.EscapeString(group.CalendarName))
		b.WriteString(renderMeetingsHTML(group.Meetings, options))
	}
	return b.String()
}

func renderBusyHTML(busy []BusyBlock, options tableFormatOptions) string {
	busyOptions := options
	busyOptions.fields = busyFields(options.fields)
	if len(busy) == 0 {
		return "<p>No busy blocks.</p>"
	}
	var b strings.Builder
	writeHTMLTable(&b, fieldLabels(busyOptions.fields), busyRows(busy, busyOptions)[1:])
	return b.String()
}

func writeHTMLTable(b *strings.Builder, headers []string, rows [][]string) {
	b.WriteString("<table><thead><tr>")
	for _, header := range headers {
		_, _ = fmt.Fprintf(b, "<th>%s</th>", html.EscapeString(header))
	}
	b.WriteString("</tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, cell := range row {
			_, _ = fmt.Fprintf(b, "<td>%s</td>", html.EscapeString(cell))
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table>")
}

func renderMeetingsMarkdown(meetings []Meeting, options tableFormatOptions) string {
	if len(meetings) == 0 {
		return "No meetings.\n"
	}
	return renderMarkdownTable(fieldLabels(options.fields), meetingRows(meetings, options)[1:])
}

func renderGroupsMarkdown(groups []CalendarMeetingGroup, options tableFormatOptions) string {
	if len(groups) == 0 {
		return "No meetings.\n"
	}
	var b strings.Builder
	for _, group := range groups {
		_, _ = fmt.Fprintf(&b, "## %s\n\n", markdownCell(group.CalendarName))
		b.WriteString(renderMeetingsMarkdown(group.Meetings, options))
		b.WriteString("\n")
	}
	return b.String()
}

func renderBusyMarkdown(busy []BusyBlock, options tableFormatOptions) string {
	busyOptions := options
	busyOptions.fields = busyFields(options.fields)
	if len(busy) == 0 {
		return "No busy blocks.\n"
	}
	return renderMarkdownTable(fieldLabels(busyOptions.fields), busyRows(busy, busyOptions)[1:])
}

func renderMarkdownTable(headers []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString("| ")
	for index, header := range headers {
		if index > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(markdownCell(header))
	}
	b.WriteString(" |\n| ")
	for index := range headers {
		if index > 0 {
			b.WriteString(" | ")
		}
		b.WriteString("---")
	}
	b.WriteString(" |\n")
	for _, row := range rows {
		b.WriteString("| ")
		for index, cell := range row {
			if index > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(markdownCell(cell))
		}
		b.WriteString(" |\n")
	}
	return b.String()
}

func markdownCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\n", " "), "|", "\\|")
}

func renderMeetingsASCII(meetings []Meeting, options tableFormatOptions) string {
	if len(meetings) == 0 {
		return "No meetings.\n"
	}
	rows := meetingRows(meetings, options)
	rows[0] = fieldLabels(options.fields)
	return renderASCIITable(rows)
}

func renderGroupsASCII(groups []CalendarMeetingGroup, options tableFormatOptions) string {
	if len(groups) == 0 {
		return "No meetings.\n"
	}
	var b strings.Builder
	for _, group := range groups {
		_, _ = fmt.Fprintf(&b, "%s\n", group.CalendarName)
		b.WriteString(renderMeetingsASCII(group.Meetings, options))
	}
	return b.String()
}

func renderBusyASCII(busy []BusyBlock, options tableFormatOptions) string {
	busyOptions := options
	busyOptions.fields = busyFields(options.fields)
	if len(busy) == 0 {
		return "No busy blocks.\n"
	}
	rows := busyRows(busy, busyOptions)
	rows[0] = fieldLabels(busyOptions.fields)
	return renderASCIITable(rows)
}

func meetingRows(meetings []Meeting, options tableFormatOptions) [][]string {
	rows := [][]string{options.fields}
	for _, meeting := range meetings {
		row := make([]string, 0, len(options.fields))
		for _, field := range options.fields {
			row = append(row, meetingField(meeting, field, options))
		}
		rows = append(rows, row)
	}
	return rows
}

func groupRows(groups []CalendarMeetingGroup, options tableFormatOptions) [][]string {
	rows := [][]string{options.fields}
	for _, group := range groups {
		for _, meeting := range group.Meetings {
			row := make([]string, 0, len(options.fields))
			for _, field := range options.fields {
				if field == "group" {
					row = append(row, group.CalendarName)
				} else {
					row = append(row, meetingField(meeting, field, options))
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func busyRows(busy []BusyBlock, options tableFormatOptions) [][]string {
	rows := [][]string{options.fields}
	for _, block := range busy {
		row := make([]string, 0, len(options.fields))
		for _, field := range options.fields {
			row = append(row, busyField(block, field, options))
		}
		rows = append(rows, row)
	}
	return rows
}

func meetingField(meeting Meeting, field string, options tableFormatOptions) string {
	switch field {
	case "when":
		return displayWhen(meeting, options)
	case "calendar":
		return meeting.CalendarName
	case "title", "name":
		return meeting.Name
	case "duration":
		return meeting.Duration
	case "duration_minutes":
		return strconv.Itoa(meeting.DurationMinutes)
	case "ongoing":
		return strconv.FormatBool(meeting.Ongoing)
	case "all_day":
		return strconv.FormatBool(meeting.AllDay)
	case "cancelled":
		return strconv.FormatBool(meeting.Cancelled)
	case "recurring":
		return strconv.FormatBool(meeting.Recurring)
	case "meeting_url":
		return meeting.MeetingURL
	case "meeting_url_type":
		return meeting.MeetingURLType
	case "description":
		return meeting.Description
	case "start":
		return meeting.Start
	case "end":
		return meeting.End
	case "timezone":
		return meeting.Timezone
	case "calendar_id":
		return meeting.CalendarID
	default:
		return ""
	}
}

func displayWhen(meeting Meeting, options tableFormatOptions) string {
	day := meeting.Day
	date := displayDate(meeting.Date)
	start := displayClock(meeting.Start)
	timeRange := displayTimeRange(meeting.Start, meeting.End)
	if timeRange == "" {
		timeRange = strings.TrimSpace(meeting.Start + "-" + meeting.End)
	}

	var value string
	switch options.timeStyle {
	case "start":
		value = strings.TrimSpace(day + " " + start)
	case "date_start":
		value = strings.TrimSpace(day + " " + date + " " + start)
	case "range":
		value = strings.TrimSpace(day + " " + timeRange)
	case "time_range":
		value = timeRange
	case "time_start":
		value = start
	default:
		value = strings.TrimSpace(day + " " + date + " " + timeRange)
	}
	if options.showTimezone && meeting.Timezone != "" {
		value = strings.TrimSpace(value + " " + meeting.Timezone)
	}
	if value != "" {
		return value
	}
	if options.showTimezone {
		return meeting.When
	}
	return stripTrailingTimezone(meeting.When)
}

func displayDate(value string) string {
	if parsed, err := time.Parse("2006-01-02", value); err == nil {
		return parsed.Format("Jan 2")
	}
	return value
}

func displayClock(value string) string {
	hour, minute, ok := parseClock(value)
	if !ok {
		return value
	}
	return fmt.Sprintf("%d:%02d %s", hour12(hour), minute, meridiem(hour))
}

func displayTimeRange(start string, end string) string {
	startHour, startMinute, okStart := parseClock(start)
	endHour, endMinute, okEnd := parseClock(end)
	if !okStart || !okEnd {
		return ""
	}
	startSuffix := meridiem(startHour)
	endSuffix := meridiem(endHour)
	if startSuffix == endSuffix {
		return fmt.Sprintf("%d:%02d-%d:%02d %s", hour12(startHour), startMinute, hour12(endHour), endMinute, endSuffix)
	}
	return fmt.Sprintf("%d:%02d %s-%d:%02d %s", hour12(startHour), startMinute, startSuffix, hour12(endHour), endMinute, endSuffix)
}

func stripTrailingTimezone(value string) string {
	fields := strings.Fields(value)
	if len(fields) < 2 {
		return value
	}
	last := fields[len(fields)-1]
	if strings.Contains(last, "/") || last == "UTC" || last == "GMT" {
		return strings.TrimSpace(strings.TrimSuffix(value, last))
	}
	return value
}

func busyField(block BusyBlock, field string, options tableFormatOptions) string {
	switch field {
	case "when":
		if options.showTimezone {
			return block.When
		}
		return stripTrailingTimezone(block.When)
	case "calendar":
		return block.Calendar
	case "duration":
		return block.Duration
	case "duration_minutes":
		return strconv.Itoa(block.DurationMinutes)
	case "ongoing":
		return strconv.FormatBool(block.Ongoing)
	case "all_day":
		return strconv.FormatBool(block.AllDay)
	default:
		return ""
	}
}

func fieldLabels(fields []string) []string {
	labels := make([]string, 0, len(fields))
	for _, field := range fields {
		labels = append(labels, fieldLabel(field))
	}
	return labels
}

func fieldLabel(field string) string {
	switch field {
	case "all_day":
		return "All Day"
	case "calendar_id":
		return "Calendar ID"
	case "duration_minutes":
		return "Duration Minutes"
	case "meeting_url":
		return "Meeting URL"
	case "meeting_url_type":
		return "Meeting URL Type"
	default:
		parts := strings.Split(field, "_")
		for index, part := range parts {
			if part == "" {
				continue
			}
			parts[index] = strings.ToUpper(part[:1]) + part[1:]
		}
		return strings.Join(parts, " ")
	}
}

func busyFields(fields []string) []string {
	allowed := map[string]bool{
		"when":             true,
		"calendar":         true,
		"duration":         true,
		"duration_minutes": true,
		"ongoing":          true,
		"all_day":          true,
	}
	filtered := []string{}
	for _, field := range fields {
		if allowed[field] {
			filtered = append(filtered, field)
		}
	}
	if len(filtered) == 0 {
		return []string{"when", "calendar", "duration"}
	}
	return filtered
}

func csvCell(value string) string {
	if strings.ContainsAny(value, "\",\n\r") {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}

func renderASCIITable(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for index, cell := range row {
			if width := len(cell); width > widths[index] {
				widths[index] = width
			}
		}
	}
	var b strings.Builder
	writeASCIIRule(&b, widths)
	for index, row := range rows {
		b.WriteString("|")
		for cellIndex, cell := range row {
			_, _ = fmt.Fprintf(&b, " %-*s |", widths[cellIndex], cell)
		}
		b.WriteString("\n")
		if index == 0 {
			writeASCIIRule(&b, widths)
		}
	}
	writeASCIIRule(&b, widths)
	return b.String()
}

func writeASCIIRule(b *strings.Builder, widths []int) {
	b.WriteString("+")
	for _, width := range widths {
		b.WriteString(strings.Repeat("-", width+2))
		b.WriteString("+")
	}
	b.WriteString("\n")
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

func openAPISpec(build ...BuildInfo) map[string]any {
	version := "dev"
	if len(build) > 0 && build[0].Version != "" {
		version = build[0].Version
	}
	response := func(description string) map[string]any {
		return map[string]any{"200": map[string]any{"description": description, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/JSONValue"}}}}}
	}
	operation := func(summary string) map[string]any {
		return map[string]any{"summary": summary, "responses": response("Successful response")}
	}
	get := func(summary string) map[string]any { return map[string]any{"get": operation(summary)} }
	post := func(summary string) map[string]any { return map[string]any{"post": operation(summary)} }
	write := func(method, summary string) map[string]any {
		op := operation(summary)
		op["requestBody"] = map[string]any{
			"required": true,
			"content": map[string]any{
				"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/JSONValue"}},
			},
		}
		return map[string]any{method: op}
	}
	meetingParameters := []map[string]any{
		{"name": "limit", "in": "query", "schema": map[string]any{"type": "integer"}, "description": "Maximum meetings to return."},
		{"name": "calendar_id", "in": "query", "schema": map[string]any{"type": "string"}, "description": "Repeat to select calendars by ID."},
		{"name": "tag", "in": "query", "schema": map[string]any{"type": "string"}, "description": "Repeat to select calendars by tag."},
		{"name": "after", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
		{"name": "before", "in": "query", "schema": map[string]any{"type": "string", "format": "date-time"}},
		{"name": "format", "in": "query", "schema": map[string]any{"type": "string", "enum": []string{"json", "html", "md", "txt", "ascii", "csv", "tg-text", "tg-html"}}, "description": "Output format; suffixes are also supported."},
	}
	eventAlias := func(summary string) map[string]any {
		op := operation(summary)
		op["description"] = "Supports shared meeting query parameters and format negotiation through `format`, `Accept`, or a `.json`, `.html`, `.md`, `.txt`, `.ascii`, or `.csv` suffix."
		op["parameters"] = meetingParameters
		return map[string]any{"get": op}
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       "ICS MCP REST API",
			"version":     version,
			"description": "Calendar reads, admin configuration, cached insights, and a REST bridge for MCP tools. Administrative operations are explicit and never run an LLM during ordinary reads.",
		},
		"paths": map[string]any{
			"/mcp":                                   post("Streamable HTTP MCP endpoint"),
			"/healthz":                               get("Liveness check"),
			"/readyz":                                get("Readiness check"),
			"/metrics":                               get("Prometheus metrics"),
			"/openapi.json":                          get("OpenAPI document"),
			"/docs":                                  get("REST API documentation page"),
			"/api/status":                            get("Service status"),
			"/api/config":                            map[string]any{"get": operation("Read runtime configuration"), "put": write("put", "Update runtime configuration")["put"]},
			"/api/environment":                       get("List recognized environment settings with secret values redacted"),
			"/api/update-check":                      get("Check latest GitHub release version"),
			"/api/llm-profile":                       map[string]any{"get": operation("Read redacted optional LLM profile"), "put": write("put", "Update optional LLM profile")["put"]},
			"/api/llm-profile/test":                  map[string]any{"post": map[string]any{"summary": "Test the effective optional LLM profile without exposing its API key"}},
			"/api/llm-profile/endpoint-test":         write("post", "Test an unsaved OpenAI-compatible endpoint"),
			"/api/llm-profile/models":                write("post", "Discover models from an unsaved OpenAI-compatible endpoint"),
			"/api/llm-profile/model-test":            write("post", "Test an unsaved OpenAI-compatible model"),
			"/api/llm-profile/lemonade/model-status": write("post", "Check whether an unsaved Lemonade model is loaded"),
			"/api/llm-profile/lemonade/model-load":   write("post", "Load and wait for an unsaved Lemonade model"),
			"/api/insight-inquiries":                 map[string]any{"get": operation("List saved insight inquiries without invoking an LLM"), "post": write("post", "Create a saved insight inquiry")["post"]},
			"/api/insight-inquiries/{name}":          map[string]any{"get": operation("Read one saved insight inquiry"), "put": write("put", "Update a saved insight inquiry")["put"], "delete": operation("Delete an inquiry template or custom inquiry and its cached output")},
			"/api/insights":                          map[string]any{"get": operation("List cached insights without invoking an LLM"), "post": write("post", "Explicitly run and cache an insight")["post"]},
			"/api/insights/preview":                  map[string]any{"post": write("post", "Explicitly test an unsaved insight without caching it")["post"]},
			"/api/insights/{name}":                   get("Read one cached insight without invoking an LLM"),
			"/api/v1/prompts":                        get("List saved prompts with latest cached outputs without invoking an LLM"),
			"/api/v1/prompts/{id}":                   get("Read one saved prompt definition"),
			"/api/v1/prompts/{id}/output":            get("Read one prompt's latest cached output without invoking an LLM"),
			"/api/v1/prompts/{id}/history":           get("Read up to ten retained prompt outcomes without invoking an LLM"),
			"/api/rest/{tool_name}":                  map[string]any{"get": operation("Call a read-only MCP tool"), "post": write("post", "Call an admin MCP tool")["post"]},
			"/api/meetings":                          eventAlias("Upcoming meetings"),
			"/api/meetings/by-calendar":              eventAlias("Upcoming meetings grouped by calendar"),
			"/api/events":                            eventAlias("Upcoming events"),
			"/api/events/by-calendar":                eventAlias("Upcoming events grouped by calendar"),
			"/api/events/today":                      eventAlias("Events that overlap the current display day"),
			"/api/events/tomorrow":                   eventAlias("Tomorrow's events"),
			"/api/events/today-tomorrow":             eventAlias("Today and tomorrow events"),
			"/api/events/today_tomorrow":             eventAlias("Today and tomorrow events"),
			"/api/events/current":                    eventAlias("Current events"),
			"/api/events/next":                       eventAlias("Next meeting-focused events"),
			"/api/events/search":                     eventAlias("Search events"),
			"/api/free-busy":                         eventAlias("Free/busy blocks"),
			"/api/tools":                             get("MCP tool catalog"),
			"/api/tools/{tool_name}/call":            write("post", "Preview an MCP tool call"),
			"/api/calendars":                         map[string]any{"get": operation("List calendar status"), "post": write("post", "Add and refresh a calendar")["post"]},
			"/api/calendars/general-query-selection": map[string]any{"get": operation("Get default-query calendar selection"), "put": write("put", "Atomically save default-query calendar selection")["put"]},
			"/api/calendars/validate":                write("post", "Validate a calendar feed"),
			"/api/calendars/{calendar}":              map[string]any{"patch": write("patch", "Update a calendar")["patch"], "delete": operation("Remove a calendar")},
			"/api/calendars/{calendar}/refresh":      post("Refresh a calendar"),
			"/api/calendars/{calendar}/events":       eventAlias("Events for one calendar"),
			"/api/calendars/{calendar}/today":        eventAlias("Events that overlap the current display day for one calendar"),
			"/api/tags":                              get("List calendar tags"),
			"/api/tags/{name}":                       map[string]any{"put": write("put", "Update a reusable calendar tag")["put"], "patch": write("patch", "Partially update a reusable calendar tag")["patch"]},
			"/{calendar}/{index}":                    eventAlias("One upcoming event for one calendar by 1-based index"),
			"/{calendar}/upcoming/{index}":           eventAlias("One upcoming event for one calendar by 1-based index"),
			"/{calendar}/ongoing/{index}":            eventAlias("One ongoing event for one calendar by 1-based index"),
		},
		"components": map[string]any{"schemas": map[string]any{
			"JSONValue": map[string]any{"description": "JSON response or request body. See the operation description and the tool catalog for concrete fields."},
			"Error":     map[string]any{"type": "object", "required": []string{"error"}, "properties": map[string]any{"error": map[string]any{"type": "string"}}},
		}},
	}
}

// openAPIDocs provides a human-readable landing page without requiring a JavaScript bundle.
func openAPIDocs(build BuildInfo) string {
	version := build.Version
	if version == "" {
		version = "dev"
	}
	return fmt.Sprintf(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ICS MCP API reference</title><style>body{max-width:960px;margin:3rem auto;padding:0 1.25rem;font:16px/1.5 system-ui,sans-serif;color:#292720;background:#faf9f5}h1{margin-bottom:.25rem}code{padding:.15rem .35rem;border-radius:.25rem;background:#ece9e1}table{width:100%%;border-collapse:collapse;margin:1.5rem 0}th,td{padding:.65rem;text-align:left;border-bottom:1px solid #ded9cf}a{color:#245b39}.note{padding:1rem;border-radius:.5rem;background:#eaf3e8}</style></head><body><h1>ICS MCP API reference</h1><p>Version <code>%s</code>. This page describes the public HTTP interface; use the <a href="/">admin console</a> for interactive configuration and the REST Explorer for meeting-format controls.</p><p class="note">Read routes never invoke an LLM. An insight runs only through its explicit <code>POST /api/insights</code> operation. Secrets are redacted from <code>/api/environment</code> and LLM-profile reads.</p><h2>Documents and service status</h2><table><tr><th>Route</th><th>Purpose</th></tr><tr><td><code>GET /openapi.json</code></td><td>Machine-readable OpenAPI 3.1 inventory.</td></tr><tr><td><code>GET /api/status</code></td><td>Build, timezone, and calendar refresh status.</td></tr><tr><td><code>GET /api/config</code>, <code>/api/environment</code></td><td>Runtime configuration and redacted environment inventory.</td></tr></table><h2>Calendar reads</h2><p><code>GET /api/events</code>, <code>/api/meetings</code>, <code>/api/free-busy</code>, and their documented aliases accept <code>limit</code>, repeated <code>calendar_id</code>/<code>tag</code>, <code>after</code>, <code>before</code>, and <code>format</code>. Calendar-scoped routes are available at <code>/api/calendars/{calendar}/events</code> and <code>/today</code>.</p><h2>Administration and insights</h2><p>Calendar and tag mutations require explicit <code>POST</code>, <code>PUT</code>, <code>PATCH</code>, or <code>DELETE</code>. Tag updates are available at <code>PUT|PATCH /api/tags/{name}</code>. Cached insight reads are <code>GET /api/insights</code>; invoke generation explicitly with <code>POST /api/insights</code>.</p><h2>MCP REST bridge</h2><p><code>GET /api/rest/{tool_name}</code> calls read-only MCP tools. <code>POST /api/rest/{tool_name}</code> is reserved for admin tools and accepts either a JSON arguments object or <code>{"arguments": {...}}</code>. Inspect <code>GET /api/tools</code> for the current catalog.</p><p><a href="/openapi.json">Open OpenAPI JSON</a> · <a href="/">Open admin console</a></p></body></html>`, version)
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

func writeText(w http.ResponseWriter, value string, err error) {
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(value))
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
}
