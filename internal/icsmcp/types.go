package icsmcp

import "time"

// Calendar is a configured ICS feed.
type Calendar struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

// CalendarStatus includes configuration plus refresh state.
type CalendarStatus struct {
	Calendar
	LastAttempt  *time.Time `json:"last_attempt,omitempty"`
	LastSuccess  *time.Time `json:"last_success,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
	NextRefresh  *time.Time `json:"next_refresh,omitempty"`
	ETag         string     `json:"etag,omitempty"`
	LastModified string     `json:"last_modified,omitempty"`
	EventCount   int        `json:"event_count"`
}

// Status is the service status payload returned by /api/status.
type Status struct {
	Now       time.Time        `json:"now"`
	Calendars []CalendarStatus `json:"calendars"`
}

// AddCalendarInput creates or upserts a calendar.
type AddCalendarInput struct {
	Key  string `json:"key,omitempty"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// UpdateCalendarInput updates mutable calendar fields.
type UpdateCalendarInput struct {
	Name    string `json:"name,omitempty"`
	URL     string `json:"url,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// EventInstance is a normalized event occurrence stored in SQLite.
type EventInstance struct {
	ID           string    `json:"id"`
	CalendarID   string    `json:"calendar_id"`
	CalendarName string    `json:"calendar_name"`
	UID          string    `json:"uid"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	Start        time.Time `json:"start_time"`
	End          time.Time `json:"end_time"`
}

// UpcomingQuery controls upcoming meeting lookup.
type UpcomingQuery struct {
	Now           time.Time `json:"-"`
	Limit         int       `json:"limit,omitempty"`
	CalendarIDs   []string  `json:"calendar_ids,omitempty"`
	LookaheadDays int       `json:"lookahead_days,omitempty"`
}

// Meeting is the MCP-facing meeting representation.
type Meeting struct {
	Day             string    `json:"day"`
	Date            string    `json:"date"`
	Start           string    `json:"start"`
	End             string    `json:"end"`
	DurationMinutes int       `json:"duration_minutes"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	CalendarID      string    `json:"calendar_id"`
	CalendarName    string    `json:"calendar_name"`
	Ongoing         bool      `json:"ongoing"`
	StartTime       time.Time `json:"-"`
}

// CalendarMeetingGroup groups upcoming meetings by calendar.
type CalendarMeetingGroup struct {
	CalendarID   string    `json:"calendar_id"`
	CalendarName string    `json:"calendar_name"`
	Meetings     []Meeting `json:"meetings"`
}
