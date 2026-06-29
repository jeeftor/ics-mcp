package icsmcp

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type meetingsOutput struct {
	Meetings []Meeting `json:"meetings"`
}

type groupedMeetingsOutput struct {
	Calendars []CalendarMeetingGroup `json:"calendars"`
}

type calendarsOutput struct {
	Calendars []CalendarStatus `json:"calendars"`
}

type calendarOutput struct {
	Calendar Calendar `json:"calendar"`
}

type okOutput struct {
	OK bool `json:"ok"`
}

type removeInput struct {
	ID string `json:"id"`
}

type refreshInput struct {
	ID string `json:"id"`
}

type updateInput struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	URL     string `json:"url,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// NewMCPServer registers calendar tools on the official Go MCP SDK server.
func NewMCPServer(svc *Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "icsmcp", Version: "v0.1.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "upcoming_meetings", Description: "List ongoing and upcoming meetings from cached ICS feeds."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, meetingsOutput, error) {
			meetings, err := svc.UpcomingMeetings(ctx, in)
			return nil, meetingsOutput{Meetings: meetings}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "upcoming_meetings_by_calendar", Description: "List ongoing and upcoming meetings grouped by calendar."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, groupedMeetingsOutput, error) {
			groups, err := svc.UpcomingMeetingsByCalendar(ctx, in)
			return nil, groupedMeetingsOutput{Calendars: groups}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "calendar_list", Description: "List configured calendars and refresh state."},
		func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, calendarsOutput, error) {
			calendars, err := svc.ListCalendarStatus(ctx)
			return nil, calendarsOutput{Calendars: calendars}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "calendar_add", Description: "Add or upsert an ICS calendar."},
		func(ctx context.Context, req *mcp.CallToolRequest, in AddCalendarInput) (*mcp.CallToolResult, calendarOutput, error) {
			cal, err := svc.AddCalendar(ctx, in)
			return nil, calendarOutput{Calendar: cal}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "calendar_update", Description: "Rename, enable, disable, or update a calendar URL."},
		func(ctx context.Context, req *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, calendarOutput, error) {
			cal, err := svc.UpdateCalendar(ctx, in.ID, UpdateCalendarInput{Name: in.Name, URL: in.URL, Enabled: in.Enabled})
			return nil, calendarOutput{Calendar: cal}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "calendar_remove", Description: "Remove a calendar and its cached events."},
		func(ctx context.Context, req *mcp.CallToolRequest, in removeInput) (*mcp.CallToolResult, okOutput, error) {
			return nil, okOutput{OK: true}, svc.RemoveCalendar(ctx, in.ID)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "calendar_refresh", Description: "Refresh a calendar feed now."},
		func(ctx context.Context, req *mcp.CallToolRequest, in refreshInput) (*mcp.CallToolResult, okOutput, error) {
			return nil, okOutput{OK: true}, svc.RefreshCalendar(ctx, in.ID, time.Now().UTC())
		})
	return server
}
