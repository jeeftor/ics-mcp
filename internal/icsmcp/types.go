package icsmcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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
	Recurring      bool      `json:"recurring"`
	RecurrenceID   string    `json:"recurrence_id"`
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
	Detail              string    `json:"detail,omitempty"`
	InProgressOnly      bool      `json:"in_progress_only,omitempty"`
	ExcludeAllDay       bool      `json:"exclude_all_day,omitempty"`
	ExcludeCancelled    bool      `json:"exclude_cancelled,omitempty"`
	After               time.Time `json:"after,omitempty"`
	Before              time.Time `json:"before,omitempty"`
	IncludeDescription  bool      `json:"include_description,omitempty"`
	DescriptionMaxChars int       `json:"description_max_chars,omitempty"`
}

// UnmarshalJSON accepts the legacy only_ongoing option while exposing in_progress_only.
func (q *UpcomingQuery) UnmarshalJSON(data []byte) error {
	type upcomingQuery UpcomingQuery
	var decoded struct {
		upcomingQuery
		LegacyOnlyOngoing bool `json:"only_ongoing,omitempty"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*q = UpcomingQuery(decoded.upcomingQuery)
	q.InProgressOnly = q.InProgressOnly || decoded.LegacyOnlyOngoing
	return nil
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
	When            string    `json:"when,omitempty"`
	Title           string    `json:"title,omitempty"`
	Calendar        string    `json:"calendar,omitempty"`
	Duration        string    `json:"duration,omitempty"`
	Day             string    `json:"day,omitempty"`
	Date            string    `json:"date,omitempty"`
	Start           string    `json:"start,omitempty"`
	End             string    `json:"end,omitempty"`
	Timezone        string    `json:"timezone,omitempty"`
	DurationMinutes int       `json:"duration_minutes"`
	Name            string    `json:"name,omitempty"`
	Description     string    `json:"description,omitempty"`
	MeetingURL      string    `json:"meeting_url,omitempty"`
	MeetingURLType  string    `json:"meeting_url_type,omitempty"`
	CalendarID      string    `json:"calendar_id,omitempty"`
	CalendarName    string    `json:"calendar_name,omitempty"`
	Ongoing         bool      `json:"ongoing"`
	AllDay          bool      `json:"all_day,omitempty"`
	Cancelled       bool      `json:"cancelled,omitempty"`
	Recurring       bool      `json:"recurring,omitempty"`
	RecurrenceID    string    `json:"recurrence_id,omitempty"`
	StartTime       time.Time `json:"-"`
	Detail          string    `json:"-"`
}

// MarshalJSON renders meetings compactly by default; detail=full keeps the verbose shape.
func (m Meeting) MarshalJSON() ([]byte, error) {
	if strings.EqualFold(m.Detail, "full") {
		type fullMeeting struct {
			Day             string `json:"day"`
			Date            string `json:"date"`
			Start           string `json:"start"`
			End             string `json:"end"`
			Timezone        string `json:"timezone"`
			DurationMinutes int    `json:"duration_minutes"`
			Name            string `json:"name"`
			Description     string `json:"description"`
			MeetingURL      string `json:"meeting_url,omitempty"`
			MeetingURLType  string `json:"meeting_url_type,omitempty"`
			CalendarID      string `json:"calendar_id"`
			CalendarName    string `json:"calendar_name"`
			Ongoing         bool   `json:"ongoing"`
			AllDay          bool   `json:"all_day"`
			Cancelled       bool   `json:"cancelled"`
			Recurring       bool   `json:"recurring"`
			RecurrenceID    string `json:"recurrence_id,omitempty"`
		}
		return json.Marshal(fullMeeting{
			Day:             m.Day,
			Date:            m.Date,
			Start:           m.Start,
			End:             m.End,
			Timezone:        m.Timezone,
			DurationMinutes: m.DurationMinutes,
			Name:            m.Name,
			Description:     m.Description,
			MeetingURL:      m.MeetingURL,
			MeetingURLType:  m.MeetingURLType,
			CalendarID:      m.CalendarID,
			CalendarName:    m.CalendarName,
			Ongoing:         m.Ongoing,
			AllDay:          m.AllDay,
			Cancelled:       m.Cancelled,
			Recurring:       m.Recurring,
			RecurrenceID:    m.RecurrenceID,
		})
	}
	type compactMeeting struct {
		When            string `json:"when"`
		Title           string `json:"title"`
		Calendar        string `json:"calendar,omitempty"`
		Duration        string `json:"duration"`
		DurationMinutes int    `json:"duration_minutes"`
		Ongoing         bool   `json:"ongoing,omitempty"`
		AllDay          bool   `json:"all_day,omitempty"`
		Cancelled       bool   `json:"cancelled,omitempty"`
		Recurring       bool   `json:"recurring,omitempty"`
		MeetingURL      string `json:"meeting_url,omitempty"`
		MeetingURLType  string `json:"meeting_url_type,omitempty"`
		Description     string `json:"description,omitempty"`
	}
	return json.Marshal(compactMeeting{
		When:            compactWhen(m),
		Title:           m.Name,
		Calendar:        m.CalendarName,
		Duration:        durationText(m.DurationMinutes),
		DurationMinutes: m.DurationMinutes,
		Ongoing:         m.Ongoing,
		AllDay:          m.AllDay,
		Cancelled:       m.Cancelled,
		Recurring:       m.Recurring,
		MeetingURL:      m.MeetingURL,
		MeetingURLType:  m.MeetingURLType,
		Description:     m.Description,
	})
}

// UnmarshalJSON accepts both compact and full meeting shapes.
func (m *Meeting) UnmarshalJSON(data []byte) error {
	type meetingAlias Meeting
	var decoded meetingAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*m = Meeting(decoded)
	if m.Name == "" {
		m.Name = m.Title
	}
	if m.CalendarName == "" {
		m.CalendarName = m.Calendar
	}
	if m.Duration == "" && m.DurationMinutes > 0 {
		m.Duration = durationText(m.DurationMinutes)
	}
	return nil
}

// CalendarMeetingGroup groups upcoming meetings by calendar.
type CalendarMeetingGroup struct {
	Calendar     string    `json:"calendar,omitempty"`
	CalendarID   string    `json:"calendar_id,omitempty"`
	CalendarName string    `json:"calendar_name,omitempty"`
	Meetings     []Meeting `json:"meetings"`
}

// BusyBlock is a token-light availability block without meeting title/details.
type BusyBlock struct {
	When            string `json:"when"`
	Calendar        string `json:"calendar,omitempty"`
	Duration        string `json:"duration"`
	DurationMinutes int    `json:"duration_minutes"`
	Ongoing         bool   `json:"ongoing,omitempty"`
	AllDay          bool   `json:"all_day,omitempty"`
}

// MarshalJSON renders grouped meeting output compactly unless the meetings requested full detail.
func (g CalendarMeetingGroup) MarshalJSON() ([]byte, error) {
	full := false
	for _, meeting := range g.Meetings {
		if strings.EqualFold(meeting.Detail, "full") {
			full = true
			break
		}
	}
	if full {
		type fullGroup CalendarMeetingGroup
		return json.Marshal(fullGroup(g))
	}
	type compactGroup struct {
		Calendar string    `json:"calendar"`
		Meetings []Meeting `json:"meetings"`
	}
	return json.Marshal(compactGroup{Calendar: g.CalendarName, Meetings: g.Meetings})
}

// UnmarshalJSON accepts both compact and full grouped meeting shapes.
func (g *CalendarMeetingGroup) UnmarshalJSON(data []byte) error {
	type groupAlias CalendarMeetingGroup
	var decoded groupAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*g = CalendarMeetingGroup(decoded)
	if g.CalendarName == "" {
		g.CalendarName = g.Calendar
	}
	if g.Calendar == "" {
		g.Calendar = g.CalendarName
	}
	return nil
}

func compactWhen(meeting Meeting) string {
	date := meeting.Date
	if parsed, err := time.Parse("2006-01-02", meeting.Date); err == nil {
		date = parsed.Format("Jan 2")
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s %s %s", meeting.Day, date, compactTimeRange(meeting.Start, meeting.End), meeting.Timezone))
}

func compactTimeRange(start string, end string) string {
	startHour, startMinute, okStart := parseClock(start)
	endHour, endMinute, okEnd := parseClock(end)
	if !okStart || !okEnd {
		return strings.TrimSpace(start + "-" + end)
	}
	startSuffix := meridiem(startHour)
	endSuffix := meridiem(endHour)
	if startSuffix == endSuffix {
		return fmt.Sprintf("%d:%02d-%d:%02d %s", hour12(startHour), startMinute, hour12(endHour), endMinute, endSuffix)
	}
	return fmt.Sprintf("%d:%02d %s-%d:%02d %s", hour12(startHour), startMinute, startSuffix, hour12(endHour), endMinute, endSuffix)
}

func parseClock(value string) (int, int, bool) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, 0, false
	}
	return parsed.Hour(), parsed.Minute(), true
}

func meridiem(hour int) string {
	if hour >= 12 {
		return "PM"
	}
	return "AM"
}

func hour12(hour int) int {
	if hour%12 == 0 {
		return 12
	}
	return hour % 12
}

func durationText(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%d min", minutes)
	}
	roundedDays := int(float64(minutes)/1440 + 0.5)
	if roundedDays > 0 {
		delta := minutes - roundedDays*1440
		if delta < 0 {
			delta = -delta
		}
		if delta <= 1 {
			if roundedDays == 1 {
				return "1 day"
			}
			return fmt.Sprintf("%d days", roundedDays)
		}
	}
	days := minutes / 1440
	hours := (minutes % 1440) / 60
	mins := minutes % 60
	parts := []string{}
	if days > 0 {
		if days == 1 {
			parts = append(parts, "1 day")
		} else {
			parts = append(parts, fmt.Sprintf("%d days", days))
		}
	}
	if hours > 0 {
		if hours == 1 {
			parts = append(parts, "1 hr")
		} else {
			parts = append(parts, fmt.Sprintf("%d hrs", hours))
		}
	}
	if mins > 0 && days == 0 {
		parts = append(parts, fmt.Sprintf("%d min", mins))
	}
	return strings.Join(parts, " ")
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
