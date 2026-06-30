package icsmcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolInfo describes an MCP tool for the admin preview UI.
type ToolInfo struct {
	Name             string         `json:"name"`
	Description      string         `json:"description"`
	Category         string         `json:"category"`
	ReadOnly         bool           `json:"read_only"`
	Destructive      bool           `json:"destructive"`
	InputExample     string         `json:"input_example"`
	DefaultArguments map[string]any `json:"default_arguments"`
}

// ToolCallRequest is a preview call payload.
type ToolCallRequest struct {
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolCallResponse is the JSON result shown by the admin preview UI.
type ToolCallResponse struct {
	Tool   string `json:"tool"`
	Result any    `json:"result"`
}

// ToolInfos returns the MCP tools exposed by this server.
func ToolInfos() []ToolInfo {
	return []ToolInfo{
		{Name: "upcoming_meetings", Description: "List ongoing and upcoming meetings by time. Compact output is default; pass detail=full for verbose fields. Descriptions are opt-in. Supports window presets, sort, include_links, links_only, and explicit disabled-calendar opt-in.", Category: "read", ReadOnly: true, InputExample: `{"limit":10,"window":"","lookahead_days":30,"timezone":"","query":"","sort":"start_time","in_progress_only":false,"exclude_all_day":false,"exclude_cancelled":true,"include_description":false,"include_links":true,"links_only":false,"include_disabled":false}`, DefaultArguments: map[string]any{"limit": 10, "window": "", "lookahead_days": 30, "timezone": "", "query": "", "sort": "start_time", "in_progress_only": false, "exclude_all_day": false, "exclude_cancelled": true, "include_description": false, "include_links": true, "links_only": false, "include_disabled": false}},
		{Name: "upcoming_meetings_by_calendar", Description: "List ongoing and upcoming meetings grouped by calendar. Compact output is default; limit applies per calendar. Sort applies within each calendar group.", Category: "read", ReadOnly: true, InputExample: `{"limit":10,"window":"","lookahead_days":30,"timezone":"","query":"","sort":"start_time","exclude_cancelled":true,"include_description":false,"include_links":true}`, DefaultArguments: map[string]any{"limit": 10, "window": "", "lookahead_days": 30, "timezone": "", "query": "", "sort": "start_time", "exclude_cancelled": true, "include_description": false, "include_links": true}},
		{Name: "next_meeting", Description: "Return the next non-all-day, non-cancelled meeting.", Category: "read", ReadOnly: true, InputExample: `{"timezone":"","include_links":true}`, DefaultArguments: map[string]any{"timezone": "", "include_links": true}},
		{Name: "next_meetings", Description: "List upcoming meeting-focused events, excluding all-day and cancelled events, using compact output by default.", Category: "read", ReadOnly: true, InputExample: `{"limit":10,"window":"","lookahead_days":30,"timezone":"","sort":"start_time","include_description":false,"include_links":true}`, DefaultArguments: map[string]any{"limit": 10, "window": "", "lookahead_days": 30, "timezone": "", "sort": "start_time", "include_description": false, "include_links": true}},
		{Name: "today_meetings", Description: "List meetings for the current display day. Defaults to agenda sort: ongoing timed meetings, upcoming timed meetings, then all-day or multi-day blocks.", Category: "read", ReadOnly: true, InputExample: `{"timezone":"","sort":"agenda","include_description":false,"include_links":true}`, DefaultArguments: map[string]any{"timezone": "", "sort": "agenda", "include_description": false, "include_links": true}},
		{Name: "current_meetings", Description: "List meetings currently in progress.", Category: "read", ReadOnly: true, InputExample: `{"exclude_all_day":true,"exclude_cancelled":true}`, DefaultArguments: map[string]any{"exclude_all_day": true, "exclude_cancelled": true}},
		{Name: "search_meetings", Description: "Search cached ongoing and upcoming meetings by title, calendar name, or cached description. Descriptions remain omitted from output unless include_description is true.", Category: "read", ReadOnly: true, InputExample: `{"query":"planning","limit":10,"window":"","sort":"start_time","exclude_cancelled":true,"include_links":true}`, DefaultArguments: map[string]any{"query": "", "limit": 10, "window": "", "sort": "start_time", "exclude_cancelled": true, "include_links": true}},
		{Name: "free_busy", Description: "List busy blocks without meeting titles or descriptions. Use window or after and before for a specific availability window.", Category: "read", ReadOnly: true, InputExample: `{"window":"today_tomorrow","after":"2026-06-30T15:00:00Z","before":"2026-07-01T00:00:00Z","limit":20,"exclude_cancelled":true,"sort":"start_time"}`, DefaultArguments: map[string]any{"window": "today", "limit": 20, "exclude_cancelled": true, "sort": "start_time"}},
		{Name: "server_status", Description: "Return server version, timezone, calendars, and refresh state.", Category: "read", ReadOnly: true, InputExample: `{}`, DefaultArguments: map[string]any{}},
		{Name: "list_calendars", Description: "List configured calendars and refresh state.", Category: "read", ReadOnly: true, InputExample: `{}`, DefaultArguments: map[string]any{}},
		{Name: "add_calendar", Description: "Add or upsert an ICS calendar and refresh it immediately.", Category: "admin", InputExample: `{"key":"WORK","name":"Work","url":"https://example.invalid/calendar.ics"}`, DefaultArguments: map[string]any{"key": "WORK", "name": "Work", "url": "https://example.invalid/calendar.ics"}},
		{Name: "validate_calendar", Description: "Fetch and parse an ICS calendar without saving it.", Category: "admin", ReadOnly: true, InputExample: `{"url":"https://example.invalid/calendar.ics","limit":5}`, DefaultArguments: map[string]any{"url": "https://example.invalid/calendar.ics", "limit": 5}},
		{Name: "update_calendar", Description: "Rename, enable, disable, update a calendar URL, or control default query inclusion.", Category: "admin", InputExample: `{"id":"calendar-id","name":"Renamed","include_in_general_queries":true}`, DefaultArguments: map[string]any{"id": "", "name": "Renamed", "include_in_general_queries": true}},
		{Name: "remove_calendar", Description: "Remove a calendar and its cached events.", Category: "admin", Destructive: true, InputExample: `{"id":"calendar-id"}`, DefaultArguments: map[string]any{"id": ""}},
		{Name: "refresh_calendar", Description: "Refresh a calendar feed now.", Category: "admin", InputExample: `{"id":"calendar-id"}`, DefaultArguments: map[string]any{"id": ""}},
		{Name: "refresh_all_calendars", Description: "Refresh all enabled calendar feeds now.", Category: "admin", InputExample: `{}`, DefaultArguments: map[string]any{}},
	}
}

// PreviewToolCall executes a tool-shaped request and returns structured JSON.
func PreviewToolCall(ctx context.Context, svc *Service, name string, raw json.RawMessage) (ToolCallResponse, error) {
	switch name {
	case "upcoming_meetings":
		var in UpcomingQuery
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		meetings, err := svc.UpcomingMeetings(ctx, in)
		return ToolCallResponse{Tool: name, Result: meetingsOutput{Meetings: meetings}}, err
	case "upcoming_meetings_by_calendar":
		var in UpcomingQuery
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		groups, err := svc.UpcomingMeetingsByCalendar(ctx, in)
		return ToolCallResponse{Tool: name, Result: groupedMeetingsOutput{Calendars: groups}}, err
	case "next_meeting":
		var in UpcomingQuery
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		meetings, err := svc.NextMeeting(ctx, in)
		return ToolCallResponse{Tool: name, Result: meetingsOutput{Meetings: meetings}}, err
	case "next_meetings":
		var in UpcomingQuery
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		in.ExcludeAllDay = true
		in.ExcludeCancelled = true
		meetings, err := svc.UpcomingMeetings(ctx, in)
		return ToolCallResponse{Tool: name, Result: meetingsOutput{Meetings: meetings}}, err
	case "today_meetings":
		var in UpcomingQuery
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		meetings, err := svc.TodayMeetings(ctx, in)
		return ToolCallResponse{Tool: name, Result: meetingsOutput{Meetings: meetings}}, err
	case "current_meetings":
		var in UpcomingQuery
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		in.InProgressOnly = true
		meetings, err := svc.UpcomingMeetings(ctx, in)
		return ToolCallResponse{Tool: name, Result: meetingsOutput{Meetings: meetings}}, err
	case "search_meetings":
		var in UpcomingQuery
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		meetings, err := svc.UpcomingMeetings(ctx, in)
		return ToolCallResponse{Tool: name, Result: meetingsOutput{Meetings: meetings}}, err
	case "free_busy":
		var in UpcomingQuery
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		busy, err := svc.FreeBusy(ctx, in)
		return ToolCallResponse{Tool: name, Result: freeBusyOutput{Busy: busy}}, err
	case "server_status":
		status, err := svc.Status(ctx)
		return ToolCallResponse{Tool: name, Result: statusOutput{Status: status}}, err
	case "list_calendars":
		calendars, err := svc.ListCalendarStatus(ctx)
		return ToolCallResponse{Tool: name, Result: calendarsOutput{Calendars: calendars}}, err
	case "add_calendar":
		var in AddCalendarInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		cal, err := svc.AddCalendarAndRefresh(ctx, in)
		return ToolCallResponse{Tool: name, Result: calendarOutput{Calendar: cal}}, err
	case "validate_calendar":
		var in ValidateCalendarInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		result, err := svc.ValidateCalendar(ctx, in)
		return ToolCallResponse{Tool: name, Result: result}, err
	case "update_calendar":
		var in updateInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		cal, err := svc.UpdateCalendar(ctx, in.ID, UpdateCalendarInput{Name: in.Name, URL: in.URL, Enabled: in.Enabled, IncludeInGeneralQueries: in.IncludeInGeneralQueries})
		return ToolCallResponse{Tool: name, Result: calendarOutput{Calendar: cal}}, err
	case "remove_calendar":
		var in removeInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		return ToolCallResponse{Tool: name, Result: okOutput{OK: true}}, svc.RemoveCalendar(ctx, in.ID)
	case "refresh_calendar":
		var in refreshInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		return ToolCallResponse{Tool: name, Result: okOutput{OK: true}}, svc.RefreshCalendar(ctx, in.ID, svc.now())
	case "refresh_all_calendars":
		results, err := svc.RefreshAllCalendars(ctx)
		return ToolCallResponse{Tool: name, Result: refreshAllOutput{Results: results}}, err
	default:
		return ToolCallResponse{}, fmt.Errorf("unknown tool %q", name)
	}
}

func decodeToolArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode tool arguments: %w", err)
	}
	return nil
}
