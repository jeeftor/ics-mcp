package icsmcp

import "time"

// Calendar is a configured ICS feed.
type Calendar struct {
	ID                      string `json:"id"`
	Key                     string `json:"key"`
	Name                    string `json:"name"`
	URL                     string `json:"url"`
	Enabled                 bool   `json:"enabled"`
	IncludeInGeneralQueries bool   `json:"include_in_general_queries"`
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

// BuildInfo describes the running binary build.
type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Status is the service status payload returned by /api/status.
type Status struct {
	Now         time.Time        `json:"now"`
	Version     BuildInfo        `json:"version"`
	Timezone    string           `json:"timezone"`
	ExternalURL string           `json:"external_url,omitempty"`
	Calendars   []CalendarStatus `json:"calendars"`
}

// AddCalendarInput creates or upserts a calendar.
type AddCalendarInput struct {
	Key  string `json:"key,omitempty"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// UpdateCalendarInput updates mutable calendar fields.
type UpdateCalendarInput struct {
	Name                    string `json:"name,omitempty"`
	URL                     string `json:"url,omitempty"`
	Enabled                 *bool  `json:"enabled,omitempty"`
	IncludeInGeneralQueries *bool  `json:"include_in_general_queries,omitempty"`
}

// CalendarSelection stores the calendars used by default generalized event queries.
type CalendarSelection struct {
	CalendarIDs []string `json:"calendar_ids"`
}

// EventInstance is a normalized event occurrence stored in SQLite.
type EventInstance struct {
	ID             string    `json:"id"`
	CalendarID     string    `json:"calendar_id"`
	CalendarName   string    `json:"calendar_name"`
	UID            string    `json:"uid"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	MeetingURL     string    `json:"meeting_url"`
	MeetingURLType string    `json:"meeting_url_type"`
	Cancelled      bool      `json:"cancelled"`
	AllDay         bool      `json:"all_day"`
	Start          time.Time `json:"start_time"`
	End            time.Time `json:"end_time"`
}

// UpcomingQuery controls upcoming meeting lookup.
type UpcomingQuery struct {
	Now                 time.Time `json:"-"`
	Limit               int       `json:"limit,omitempty"`
	CalendarIDs         []string  `json:"calendar_ids,omitempty"`
	LookaheadDays       int       `json:"lookahead_days,omitempty"`
	Query               string    `json:"query,omitempty"`
	Timezone            string    `json:"timezone,omitempty"`
	OnlyOngoing         bool      `json:"only_ongoing,omitempty"`
	ExcludeAllDay       bool      `json:"exclude_all_day,omitempty"`
	ExcludeCancelled    bool      `json:"exclude_cancelled,omitempty"`
	After               time.Time `json:"after,omitempty"`
	Before              time.Time `json:"before,omitempty"`
	IncludeDescription  bool      `json:"include_description,omitempty"`
	DescriptionMaxChars int       `json:"description_max_chars,omitempty"`
}

// upcomingDefaults resolves default query limits.
func (q UpcomingQuery) limit(defaultLimit int) int {
	if q.Limit > 0 {
		return q.Limit
	}
	return defaultLimit
}

// Meeting is the MCP-facing meeting representation.
type Meeting struct {
	Day             string    `json:"day"`
	Date            string    `json:"date"`
	Start           string    `json:"start"`
	End             string    `json:"end"`
	Timezone        string    `json:"timezone"`
	DurationMinutes int       `json:"duration_minutes"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	MeetingURL      string    `json:"meeting_url,omitempty"`
	MeetingURLType  string    `json:"meeting_url_type,omitempty"`
	CalendarID      string    `json:"calendar_id"`
	CalendarName    string    `json:"calendar_name"`
	Ongoing         bool      `json:"ongoing"`
	AllDay          bool      `json:"all_day"`
	Cancelled       bool      `json:"cancelled"`
	StartTime       time.Time `json:"-"`
}

// CalendarMeetingGroup groups upcoming meetings by calendar.
type CalendarMeetingGroup struct {
	CalendarID   string    `json:"calendar_id"`
	CalendarName string    `json:"calendar_name"`
	Meetings     []Meeting `json:"meetings"`
}

// ValidateCalendarInput checks an ICS feed without saving it.
type ValidateCalendarInput struct {
	URL           string `json:"url"`
	LookaheadDays int    `json:"lookahead_days,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

// ValidateCalendarResult summarizes a feed validation attempt.
type ValidateCalendarResult struct {
	OK         bool      `json:"ok"`
	StatusCode int       `json:"status_code,omitempty"`
	EventCount int       `json:"event_count"`
	Meetings   []Meeting `json:"meetings,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// RefreshCalendarResult summarizes a manual calendar refresh.
type RefreshCalendarResult struct {
	CalendarID   string `json:"calendar_id"`
	CalendarName string `json:"calendar_name"`
	OK           bool   `json:"ok"`
	Skipped      bool   `json:"skipped,omitempty"`
	Error        string `json:"error,omitempty"`
}
