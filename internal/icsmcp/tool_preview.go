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
		{Name: "upcoming_meetings", Description: "List ongoing and upcoming meetings by time. Supports query, time, ongoing, and all-day filters; descriptions are opt-in.", Category: "read", ReadOnly: true, InputExample: `{"limit":10,"lookahead_days":30,"query":"","only_ongoing":false,"exclude_all_day":false,"include_description":false}`, DefaultArguments: map[string]any{"limit": 10, "lookahead_days": 30, "query": "", "only_ongoing": false, "exclude_all_day": false, "include_description": false}},
		{Name: "upcoming_meetings_by_calendar", Description: "List ongoing and upcoming meetings grouped by calendar. Limit applies per calendar; descriptions are opt-in.", Category: "read", ReadOnly: true, InputExample: `{"limit":10,"lookahead_days":30,"query":"","include_description":false}`, DefaultArguments: map[string]any{"limit": 10, "lookahead_days": 30, "query": "", "include_description": false}},
		{Name: "calendar_list", Description: "List configured calendars and refresh state.", Category: "read", ReadOnly: true, InputExample: `{}`, DefaultArguments: map[string]any{}},
		{Name: "calendar_add", Description: "Add or upsert an ICS calendar.", Category: "admin", InputExample: `{"key":"WORK","name":"Work","url":"https://example.invalid/calendar.ics"}`, DefaultArguments: map[string]any{"key": "WORK", "name": "Work", "url": "https://example.invalid/calendar.ics"}},
		{Name: "calendar_validate", Description: "Fetch and parse an ICS calendar without saving it.", Category: "admin", ReadOnly: true, InputExample: `{"url":"https://example.invalid/calendar.ics","limit":5}`, DefaultArguments: map[string]any{"url": "https://example.invalid/calendar.ics", "limit": 5}},
		{Name: "calendar_update", Description: "Rename, enable, disable, or update a calendar URL.", Category: "admin", InputExample: `{"id":"calendar-id","name":"Renamed"}`, DefaultArguments: map[string]any{"id": "", "name": "Renamed"}},
		{Name: "calendar_remove", Description: "Remove a calendar and its cached events.", Category: "admin", Destructive: true, InputExample: `{"id":"calendar-id"}`, DefaultArguments: map[string]any{"id": ""}},
		{Name: "calendar_refresh", Description: "Refresh a calendar feed now.", Category: "admin", InputExample: `{"id":"calendar-id"}`, DefaultArguments: map[string]any{"id": ""}},
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
	case "calendar_list":
		calendars, err := svc.ListCalendarStatus(ctx)
		return ToolCallResponse{Tool: name, Result: calendarsOutput{Calendars: calendars}}, err
	case "calendar_add":
		var in AddCalendarInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		cal, err := svc.AddCalendar(ctx, in)
		return ToolCallResponse{Tool: name, Result: calendarOutput{Calendar: cal}}, err
	case "calendar_validate":
		var in ValidateCalendarInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		result, err := svc.ValidateCalendar(ctx, in)
		return ToolCallResponse{Tool: name, Result: result}, err
	case "calendar_update":
		var in updateInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		cal, err := svc.UpdateCalendar(ctx, in.ID, UpdateCalendarInput{Name: in.Name, URL: in.URL, Enabled: in.Enabled})
		return ToolCallResponse{Tool: name, Result: calendarOutput{Calendar: cal}}, err
	case "calendar_remove":
		var in removeInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		return ToolCallResponse{Tool: name, Result: okOutput{OK: true}}, svc.RemoveCalendar(ctx, in.ID)
	case "calendar_refresh":
		var in refreshInput
		if err := decodeToolArgs(raw, &in); err != nil {
			return ToolCallResponse{}, err
		}
		return ToolCallResponse{Tool: name, Result: okOutput{OK: true}}, svc.RefreshCalendar(ctx, in.ID, svc.now())
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
