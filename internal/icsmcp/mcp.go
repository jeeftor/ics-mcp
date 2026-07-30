package icsmcp

import (
	"context"
	"encoding/json"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type meetingsOutput struct {
	Meetings []Meeting `json:"meetings"`
	Text     string    `json:"text,omitempty"`
}

type groupedMeetingsOutput struct {
	Calendars []CalendarMeetingGroup `json:"calendars"`
	Text      string                 `json:"text,omitempty"`
}

type freeBusyOutput struct {
	Busy []BusyBlock `json:"busy"`
	Text string      `json:"text,omitempty"`
}

type meetingOutput struct {
	Meeting Meeting `json:"meeting"`
	Text    string  `json:"text,omitempty"`
}

type calendarMeetingInput struct {
	UpcomingQuery
	Calendar string `json:"calendar"`
	Index    int    `json:"index"`
	List     string `json:"list,omitempty"`
}

func (in *calendarMeetingInput) UnmarshalJSON(data []byte) error {
	var query UpcomingQuery
	if err := json.Unmarshal(data, &query); err != nil {
		return err
	}
	var fields struct {
		Calendar string `json:"calendar"`
		Index    int    `json:"index"`
		List     string `json:"list,omitempty"`
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	*in = calendarMeetingInput{
		UpcomingQuery: query,
		Calendar:      fields.Calendar,
		Index:         fields.Index,
		List:          fields.List,
	}
	return nil
}

type calendarsOutput struct {
	Calendars []CalendarStatus `json:"calendars"`
}

type tagsOutput struct {
	Tags []CalendarTag `json:"tags"`
}

type configOutput struct {
	Config RuntimeConfig `json:"config"`
}

type statusOutput struct {
	Status Status `json:"status"`
}

type calendarOutput struct {
	Calendar Calendar `json:"calendar"`
}

type refreshAllOutput struct {
	Results []RefreshCalendarResult `json:"results"`
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
	ID                      string    `json:"id"`
	Name                    string    `json:"name,omitempty"`
	URL                     string    `json:"url,omitempty"`
	Enabled                 *bool     `json:"enabled,omitempty"`
	IncludeInGeneralQueries *bool     `json:"include_in_general_queries,omitempty"`
	Tags                    *[]string `json:"tags,omitempty"`
}

// NewMCPServer registers calendar tools on the official Go MCP SDK server.
func NewMCPServer(svc *Service) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "icsmcp", Version: svc.buildInfo.Version}, nil)
	registerMCPResources(server, svc)
	registerMCPPrompts(server)
	mcp.AddTool(server, &mcp.Tool{Name: "upcoming_meetings", Description: "List ongoing and upcoming meetings from cached ICS feeds. Omit fields for compact default output; pass fields only to override structured fields. Supports window presets, sort, include_links, links_only, and format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, meetingsOutput, error) {
			meetings, err := svc.UpcomingMeetings(ctx, in)
			out, formatErr := newMeetingsOutput(meetings, in)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "upcoming_meetings_by_calendar", Description: "List ongoing and upcoming meetings grouped by calendar. Omit fields for compact default output; pass fields only to override each meeting. Limit applies per calendar; sort applies within each group. Supports format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, groupedMeetingsOutput, error) {
			groups, err := svc.UpcomingMeetingsByCalendar(ctx, in)
			out, formatErr := newGroupedMeetingsOutput(groups, in)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "next_meeting", Description: "Return the next non-all-day, non-cancelled meeting. Omit fields for compact default output; pass fields only to override structured fields. Supports format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, meetingsOutput, error) {
			meetings, err := svc.NextMeeting(ctx, in)
			out, formatErr := newMeetingsOutput(meetings, in)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "next_meetings", Description: "List upcoming meeting-focused events, excluding all-day and cancelled events. Omit fields for compact default output; pass fields only to override structured fields. Supports format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, meetingsOutput, error) {
			in.ExcludeAllDay = true
			in.ExcludeCancelled = true
			meetings, err := svc.UpcomingMeetings(ctx, in)
			out, formatErr := newMeetingsOutput(meetings, in)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "today_meetings", Description: "List meetings that overlap the current display day. Includes today's timed meetings, today's all-day blocks, and ongoing multi-day events, but ignores broader window/day/range presets so tomorrow and later events are not returned. Defaults to agenda sort. Omit fields for compact default output; pass fields only to override structured fields. Supports format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, meetingsOutput, error) {
			meetings, err := svc.TodayMeetings(ctx, in)
			out, formatErr := newMeetingsOutput(meetings, in)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "current_meetings", Description: "List meetings that are currently in progress. Omit fields for compact default output; pass fields only to override structured fields. Supports format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, meetingsOutput, error) {
			in.InProgressOnly = true
			meetings, err := svc.UpcomingMeetings(ctx, in)
			out, formatErr := newMeetingsOutput(meetings, in)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "search_meetings", Description: "Search cached ongoing and upcoming meetings by title, calendar name, or cached description. Omit fields for compact default output; pass fields only to override structured fields. Descriptions remain omitted from output unless requested. Supports format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, meetingsOutput, error) {
			meetings, err := svc.UpcomingMeetings(ctx, in)
			out, formatErr := newMeetingsOutput(meetings, in)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "calendar_meeting", Description: "Return one meeting from one calendar by 1-based index. Set calendar to a calendar ID or key. list defaults to upcoming and also accepts ongoing. Omit fields for compact default output; pass fields only to override structured fields. Supports format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in calendarMeetingInput) (*mcp.CallToolResult, meetingOutput, error) {
			meeting, err := svc.CalendarMeeting(ctx, in)
			out, formatErr := newMeetingOutput(meeting, in.UpcomingQuery)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "free_busy", Description: "List busy blocks without meeting titles or descriptions. Omit fields for compact default busy-block output; pass fields only to override structured busy fields. Use window presets or after and before for a specific availability window. Supports format=tg-text/tg-html/tg-markdownv2."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpcomingQuery) (*mcp.CallToolResult, freeBusyOutput, error) {
			busy, err := svc.FreeBusy(ctx, in)
			out, formatErr := newFreeBusyOutput(busy, in)
			return nil, out, firstError(err, formatErr)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "server_status", Description: "Return server version, timezone, calendars, and refresh state."},
		func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, statusOutput, error) {
			status, err := svc.Status(ctx)
			return nil, statusOutput{Status: status}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "list_calendars", Description: "List configured calendars and refresh state."},
		func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, calendarsOutput, error) {
			calendars, err := svc.ListCalendarStatus(ctx)
			return nil, calendarsOutput{Calendars: calendars}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "list_tags", Description: "List calendar tags and how many calendars use each tag."},
		func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, tagsOutput, error) {
			tags, err := svc.ListTags(ctx)
			return nil, tagsOutput{Tags: tags}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "get_config", Description: "Return effective runtime configuration and each setting source."},
		func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, configOutput, error) {
			return nil, configOutput{Config: svc.RuntimeConfig()}, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "update_config", Description: "Persist and apply unlocked runtime configuration settings."},
		func(ctx context.Context, req *mcp.CallToolRequest, in UpdateRuntimeConfigInput) (*mcp.CallToolResult, configOutput, error) {
			config, err := svc.UpdateRuntimeConfig(ctx, in)
			return nil, configOutput{Config: config}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "add_calendar", Description: "Add or upsert an ICS calendar."},
		func(ctx context.Context, req *mcp.CallToolRequest, in AddCalendarInput) (*mcp.CallToolResult, calendarOutput, error) {
			cal, err := svc.AddCalendarAndRefresh(ctx, in)
			return nil, calendarOutput{Calendar: cal}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "update_calendar", Description: "Rename, enable, disable, update a calendar URL, or control default query inclusion."},
		func(ctx context.Context, req *mcp.CallToolRequest, in updateInput) (*mcp.CallToolResult, calendarOutput, error) {
			cal, err := svc.UpdateCalendar(ctx, in.ID, UpdateCalendarInput{Name: in.Name, URL: in.URL, Enabled: in.Enabled, IncludeInGeneralQueries: in.IncludeInGeneralQueries, Tags: in.Tags})
			return nil, calendarOutput{Calendar: cal}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "remove_calendar", Description: "Remove a calendar and its cached events."},
		func(ctx context.Context, req *mcp.CallToolRequest, in removeInput) (*mcp.CallToolResult, okOutput, error) {
			return nil, okOutput{OK: true}, svc.RemoveCalendar(ctx, in.ID)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "refresh_calendar", Description: "Refresh a calendar feed now."},
		func(ctx context.Context, req *mcp.CallToolRequest, in refreshInput) (*mcp.CallToolResult, okOutput, error) {
			return nil, okOutput{OK: true}, svc.RefreshCalendar(ctx, in.ID, time.Now().UTC())
		})
	mcp.AddTool(server, &mcp.Tool{Name: "refresh_all_calendars", Description: "Refresh all enabled calendar feeds now."},
		func(ctx context.Context, req *mcp.CallToolRequest, in any) (*mcp.CallToolResult, refreshAllOutput, error) {
			results, err := svc.RefreshAllCalendars(ctx)
			return nil, refreshAllOutput{Results: results}, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "validate_calendar", Description: "Fetch and parse an ICS feed without saving it."},
		func(ctx context.Context, req *mcp.CallToolRequest, in ValidateCalendarInput) (*mcp.CallToolResult, ValidateCalendarResult, error) {
			result, err := svc.ValidateCalendar(ctx, in)
			return nil, result, err
		})
	return server
}

func newMeetingsOutput(meetings []Meeting, query UpcomingQuery) (meetingsOutput, error) {
	projected, err := meetingsWithFields(meetings, query.Fields)
	if err != nil {
		return meetingsOutput{}, err
	}
	text, err := FormatMeetings(meetings, query.Format)
	return meetingsOutput{Meetings: projected, Text: text}, err
}

func newMeetingOutput(meeting Meeting, query UpcomingQuery) (meetingOutput, error) {
	if len(query.Fields) > 0 {
		meeting.Fields = query.Fields
		if _, err := projectMeeting(meeting); err != nil {
			return meetingOutput{}, err
		}
	}
	text, err := FormatMeetings([]Meeting{meeting}, query.Format)
	return meetingOutput{Meeting: meeting, Text: text}, err
}

func newGroupedMeetingsOutput(groups []CalendarMeetingGroup, query UpcomingQuery) (groupedMeetingsOutput, error) {
	projected, err := groupsWithFields(groups, query.Fields)
	if err != nil {
		return groupedMeetingsOutput{}, err
	}
	text, err := FormatGroupedMeetings(groups, query.Format)
	return groupedMeetingsOutput{Calendars: projected, Text: text}, err
}

func newFreeBusyOutput(busy []BusyBlock, query UpcomingQuery) (freeBusyOutput, error) {
	projected, err := busyWithFields(busy, query.Fields)
	if err != nil {
		return freeBusyOutput{}, err
	}
	text, err := FormatBusyBlocks(busy, query.Format)
	return freeBusyOutput{Busy: projected, Text: text}, err
}

func meetingsWithFields(meetings []Meeting, fields []string) ([]Meeting, error) {
	if len(fields) == 0 {
		return meetings, nil
	}
	projected := make([]Meeting, len(meetings))
	for index, meeting := range meetings {
		meeting.Fields = fields
		if _, err := projectMeeting(meeting); err != nil {
			return nil, err
		}
		projected[index] = meeting
	}
	return projected, nil
}

func groupsWithFields(groups []CalendarMeetingGroup, fields []string) ([]CalendarMeetingGroup, error) {
	if len(fields) == 0 {
		return groups, nil
	}
	projected := make([]CalendarMeetingGroup, len(groups))
	for groupIndex, group := range groups {
		meetings, err := meetingsWithFields(group.Meetings, fields)
		if err != nil {
			return nil, err
		}
		group.Meetings = meetings
		projected[groupIndex] = group
	}
	return projected, nil
}

func busyWithFields(busy []BusyBlock, fields []string) ([]BusyBlock, error) {
	if len(fields) == 0 {
		return busy, nil
	}
	projected := make([]BusyBlock, len(busy))
	for index, block := range busy {
		block.Fields = fields
		if _, err := projectBusyBlock(block); err != nil {
			return nil, err
		}
		projected[index] = block
	}
	return projected, nil
}

func firstError(err error, next error) error {
	if err != nil {
		return err
	}
	return next
}
