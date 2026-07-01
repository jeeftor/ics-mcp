package icsmcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	resourceStatus           = "icsmcp://status"
	resourceCalendars        = "icsmcp://calendars"
	resourceTodayMeetings    = "icsmcp://meetings/today"
	resourceUpcomingMeetings = "icsmcp://meetings/upcoming"
)

func registerMCPResources(server *mcp.Server, svc *Service) {
	server.AddResource(&mcp.Resource{Name: "status", Description: "Server version, timezone, external URL, calendars, and refresh state.", MIMEType: "application/json", URI: resourceStatus},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			status, err := svc.Status(ctx)
			return jsonResource(req.Params.URI, statusOutput{Status: status}, err)
		})
	server.AddResource(&mcp.Resource{Name: "calendars", Description: "Configured calendars and refresh state.", MIMEType: "application/json", URI: resourceCalendars},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			calendars, err := svc.ListCalendarStatus(ctx)
			return jsonResource(req.Params.URI, calendarsOutput{Calendars: calendars}, err)
		})
	server.AddResource(&mcp.Resource{Name: "today_meetings", Description: "Meetings that overlap the current display day using agenda sort.", MIMEType: "application/json", URI: resourceTodayMeetings},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			meetings, err := svc.TodayMeetings(ctx, UpcomingQuery{Sort: "agenda"})
			return jsonResource(req.Params.URI, meetingsOutput{Meetings: meetings}, err)
		})
	server.AddResource(&mcp.Resource{Name: "upcoming_meetings", Description: "Next 10 upcoming meetings using compact start-time order.", MIMEType: "application/json", URI: resourceUpcomingMeetings},
		func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			meetings, err := svc.UpcomingMeetings(ctx, UpcomingQuery{Limit: 10, Sort: "start_time", ExcludeCancelled: true})
			return jsonResource(req.Params.URI, meetingsOutput{Meetings: meetings}, err)
		})
}

func jsonResource(uri string, value any, err error) (*mcp.ReadResourceResult, error) {
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal resource %s: %w", uri, err)
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func registerMCPPrompts(server *mcp.Server) {
	server.AddPrompt(&mcp.Prompt{
		Name:        "daily_briefing",
		Title:       "Daily Briefing",
		Description: "Summarize today's calendar and call out what is next.",
	}, staticPrompt("Daily Briefing", "Read icsmcp://meetings/today or call today_meetings with sort=agenda. Treat the result as the current display day only: it can include meetings that overlap today, including ongoing multi-day blocks, but should not include tomorrow or later events. Summarize ongoing meetings, next timed meetings, all-day or multi-day blocks, and any available join links. Keep it concise and action-oriented."))

	server.AddPrompt(&mcp.Prompt{
		Name:        "meeting_prep",
		Title:       "Meeting Prep",
		Description: "Prepare for the next relevant meeting.",
		Arguments: []*mcp.PromptArgument{
			{Name: "query", Description: "Optional title, calendar, or description text to search for.", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		query := req.Params.Arguments["query"]
		text := "Call next_meeting first. If a query is supplied, call search_meetings with that query, include_description=true, and limit=3. Use meeting_url when present and summarize the title, time, calendar, duration, and useful preparation notes."
		if query != "" {
			text += " Query: " + query
		}
		return promptResult("Meeting Prep", text), nil
	})

	server.AddPrompt(&mcp.Prompt{
		Name:        "availability_summary",
		Title:       "Availability Summary",
		Description: "Summarize busy/free blocks for a time window.",
		Arguments: []*mcp.PromptArgument{
			{Name: "window", Description: "Optional preset such as today, tomorrow, today_tomorrow, next_24h, rest_of_week, or rest_of_work_week.", Required: false},
		},
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		window := req.Params.Arguments["window"]
		if window == "" {
			window = "today"
		}
		return promptResult("Availability Summary", "Call free_busy with window="+window+", sort=start_time, and exclude_cancelled=true. Summarize busy blocks without exposing titles unless another tool call is needed."), nil
	})

	server.AddPrompt(&mcp.Prompt{
		Name:        "calendar_debug_report",
		Title:       "Calendar Debug Report",
		Description: "Inspect server status, calendar config, and refresh health.",
	}, staticPrompt("Calendar Debug Report", "Read icsmcp://status and icsmcp://calendars. Report version, timezone, configured calendars, event counts, last success, next refresh, and any refresh errors."))
}

func staticPrompt(description string, text string) mcp.PromptHandler {
	return func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return promptResult(description, text), nil
	}
}

func promptResult(description string, text string) *mcp.GetPromptResult {
	return &mcp.GetPromptResult{
		Description: description,
		Messages: []*mcp.PromptMessage{{
			Role:    "user",
			Content: &mcp.TextContent{Text: text},
		}},
	}
}
