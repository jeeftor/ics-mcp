package icsmcp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ServiceOptions configures Service behavior.
type ServiceOptions struct {
	RefreshInterval time.Duration
	Lookahead       time.Duration
	HTTPClient      *http.Client
	Logger          *slog.Logger
	BuildInfo       BuildInfo
	Timezone        string
	ExternalURL     string
}

// Service coordinates calendar config, refreshes, and meeting queries.
type Service struct {
	store           *Store
	refreshInterval time.Duration
	lookahead       time.Duration
	httpClient      *http.Client
	logger          *slog.Logger
	buildInfo       BuildInfo
	location        *time.Location
	timezone        string
	externalURL     string
	clockMu         sync.RWMutex
	clock           func() time.Time
}

// NewService constructs a calendar service.
func NewService(store *Store, opts ServiceOptions) *Service {
	if opts.RefreshInterval == 0 {
		opts.RefreshInterval = 5 * time.Minute
	}
	if opts.Lookahead == 0 {
		opts.Lookahead = 30 * 24 * time.Hour
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	location, timezone := resolveLocation(opts.Timezone, opts.Logger)
	return &Service{
		store:           store,
		refreshInterval: opts.RefreshInterval,
		lookahead:       opts.Lookahead,
		httpClient:      opts.HTTPClient,
		logger:          opts.Logger,
		buildInfo:       normalizeBuildInfo(opts.BuildInfo),
		location:        location,
		timezone:        timezone,
		externalURL:     normalizeExternalURL(opts.ExternalURL),
		clock:           time.Now,
	}
}

func normalizeExternalURL(value string) string {
	return strings.TrimRight(strings.TrimSpace(value), "/")
}

func resolveLocation(value string, logger *slog.Logger) (*time.Location, string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.UTC, "UTC"
	}
	location, timezone, err := loadLocation(value)
	if err != nil {
		logger.Warn("timezone not recognized, defaulting to UTC", "timezone", value, "error", err)
		return time.UTC, "UTC"
	}
	return location, timezone
}

func normalizeBuildInfo(in BuildInfo) BuildInfo {
	if in.Version == "" {
		in.Version = "dev"
	}
	if in.Commit == "" {
		in.Commit = "unknown"
	}
	if in.Date == "" {
		in.Date = "unknown"
	}
	return in
}

// SetBuildInfo replaces build metadata for tests.
func (s *Service) SetBuildInfo(info BuildInfo) {
	s.buildInfo = normalizeBuildInfo(info)
}

// SetClock replaces the service clock for tests.
func (s *Service) SetClock(clock func() time.Time) {
	s.clockMu.Lock()
	defer s.clockMu.Unlock()
	s.clock = clock
}

func (s *Service) now() time.Time {
	s.clockMu.RLock()
	defer s.clockMu.RUnlock()
	return s.clock().UTC()
}

// ImportStartupCalendars imports environment and CLI calendars without deleting UI-added calendars.
func (s *Service) ImportStartupCalendars(ctx context.Context, env map[string]string, cli []string) error {
	for _, cal := range calendarsFromEnv(env) {
		if _, err := s.store.upsertCalendar(ctx, cal, true); err != nil {
			return err
		}
		s.logger.Info("imported startup calendar from environment", "calendar_id", cal.ID, "key", cal.Key, "name", cal.Name)
	}
	for _, value := range cli {
		cal, err := calendarFromAssignment(value)
		if err != nil {
			return err
		}
		if _, err := s.store.upsertCalendar(ctx, cal, true); err != nil {
			return err
		}
		s.logger.Info("imported startup calendar from cli", "calendar_id", cal.ID, "key", cal.Key, "name", cal.Name)
	}
	return nil
}

// AddCalendar creates or updates a calendar.
func (s *Service) AddCalendar(ctx context.Context, in AddCalendarInput) (Calendar, error) {
	cal, err := calendarFromInput(in)
	if err != nil {
		return Calendar{}, err
	}
	saved, err := s.store.upsertCalendar(ctx, cal, false)
	if err != nil {
		return Calendar{}, err
	}
	s.logger.Info("calendar saved", "calendar_id", saved.ID, "key", saved.Key, "name", saved.Name, "enabled", saved.Enabled)
	return saved, nil
}

// AddCalendarAndRefresh creates or updates a calendar and attempts an immediate refresh.
func (s *Service) AddCalendarAndRefresh(ctx context.Context, in AddCalendarInput) (Calendar, error) {
	cal, err := s.AddCalendar(ctx, in)
	if err != nil {
		return Calendar{}, err
	}
	if err := s.RefreshCalendar(ctx, cal.ID, s.now()); err != nil {
		s.logger.Warn("calendar add refresh failed", "calendar_id", cal.ID, "key", cal.Key, "name", cal.Name, "error", err)
	}
	return cal, nil
}

// ListCalendars returns configured calendars.
func (s *Service) ListCalendars(ctx context.Context) ([]Calendar, error) {
	return s.store.listCalendars(ctx)
}

// ListCalendarStatus returns calendars with refresh state.
func (s *Service) ListCalendarStatus(ctx context.Context) ([]CalendarStatus, error) {
	return s.store.listCalendarStatus(ctx)
}

// UpdateCalendar updates a calendar by ID.
func (s *Service) UpdateCalendar(ctx context.Context, id string, in UpdateCalendarInput) (Calendar, error) {
	return s.store.updateCalendar(ctx, id, in)
}

// GeneralQueryCalendars returns calendar IDs included in default generalized meeting queries.
func (s *Service) GeneralQueryCalendars(ctx context.Context) (CalendarSelection, error) {
	calendars, err := s.ListCalendars(ctx)
	if err != nil {
		return CalendarSelection{}, err
	}
	ids := make([]string, 0, len(calendars))
	for _, cal := range calendars {
		if cal.IncludeInGeneralQueries {
			ids = append(ids, cal.ID)
		}
	}
	return CalendarSelection{CalendarIDs: ids}, nil
}

// SetGeneralQueryCalendars saves calendar IDs included in default generalized meeting queries.
func (s *Service) SetGeneralQueryCalendars(ctx context.Context, calendarIDs []string) (CalendarSelection, error) {
	ids := uniqueCalendarIDs(calendarIDs)
	if err := s.store.setGeneralQueryCalendarIDs(ctx, ids); err != nil {
		return CalendarSelection{}, err
	}
	s.logger.Info("calendar default query selection saved", "calendar_count", len(ids))
	return s.GeneralQueryCalendars(ctx)
}

// RemoveCalendar deletes a calendar and cached events.
func (s *Service) RemoveCalendar(ctx context.Context, id string) error {
	if err := s.store.deleteCalendar(ctx, id); err != nil {
		return err
	}
	s.logger.Info("calendar removed", "calendar_id", id)
	return nil
}

// ReplaceEvents replaces cached instances for a calendar.
func (s *Service) ReplaceEvents(ctx context.Context, calendarID string, events []EventInstance) error {
	for i := range events {
		if events[i].ID == "" {
			events[i].ID = uuid.NewString()
		}
		events[i].CalendarID = calendarID
	}
	return s.store.replaceEvents(ctx, calendarID, events)
}

// RefreshCalendar fetches and parses a calendar, preserving cached events on failures.
func (s *Service) RefreshCalendar(ctx context.Context, id string, now time.Time) error {
	cal, err := s.store.calendarByID(ctx, id)
	if err != nil {
		return err
	}
	s.logger.Debug("calendar refresh starting", "calendar_id", cal.ID, "key", cal.Key, "name", cal.Name)
	state, err := s.store.refreshState(ctx, id)
	if err != nil {
		return err
	}
	attempt := now.UTC()
	state.LastAttempt = &attempt
	next := attempt.Add(s.refreshInterval)
	state.NextRefresh = &next

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cal.URL, nil)
	if err != nil {
		state.LastError = err.Error()
		_ = s.store.updateRefreshState(ctx, id, state)
		s.logger.Warn("calendar refresh failed", "calendar_id", cal.ID, "key", cal.Key, "error", err)
		return err
	}
	if state.ETag != "" {
		req.Header.Set("If-None-Match", state.ETag)
	}
	if state.LastModified != "" {
		req.Header.Set("If-Modified-Since", state.LastModified)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		state.LastError = err.Error()
		_ = s.store.updateRefreshState(ctx, id, state)
		s.logger.Warn("calendar refresh failed", "calendar_id", cal.ID, "key", cal.Key, "error", err)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		success := attempt
		state.LastSuccess = &success
		state.LastError = ""
		s.logger.Info("calendar refresh not modified", "calendar_id", cal.ID, "key", cal.Key, "name", cal.Name)
		return s.store.updateRefreshState(ctx, id, state)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err := fmt.Errorf("fetch %s: status %d", cal.URL, resp.StatusCode)
		state.LastError = err.Error()
		_ = s.store.updateRefreshState(ctx, id, state)
		s.logger.Warn("calendar refresh failed", "calendar_id", cal.ID, "key", cal.Key, "status", resp.StatusCode)
		return err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		state.LastError = err.Error()
		_ = s.store.updateRefreshState(ctx, id, state)
		s.logger.Warn("calendar refresh failed", "calendar_id", cal.ID, "key", cal.Key, "error", err)
		return err
	}
	events, err := ParseICS(string(body), attempt, s.lookahead)
	if err != nil {
		state.LastError = err.Error()
		_ = s.store.updateRefreshState(ctx, id, state)
		s.logger.Warn("calendar refresh failed", "calendar_id", cal.ID, "key", cal.Key, "error", err)
		return err
	}
	for i := range events {
		events[i].CalendarID = cal.ID
		events[i].CalendarName = cal.Name
	}
	if err := s.ReplaceEvents(ctx, id, events); err != nil {
		state.LastError = err.Error()
		_ = s.store.updateRefreshState(ctx, id, state)
		s.logger.Warn("calendar refresh failed", "calendar_id", cal.ID, "key", cal.Key, "error", err)
		return err
	}
	success := attempt
	state.LastSuccess = &success
	state.LastError = ""
	state.ETag = resp.Header.Get("ETag")
	state.LastModified = resp.Header.Get("Last-Modified")
	state.EventCount = len(events)
	s.logger.Info("calendar refresh succeeded", "calendar_id", cal.ID, "key", cal.Key, "name", cal.Name, "event_count", len(events), "next_refresh", next.Format(time.RFC3339))
	return s.store.updateRefreshState(ctx, id, state)
}

// RefreshDueCalendars refreshes enabled calendars whose next refresh has arrived.
func (s *Service) RefreshDueCalendars(ctx context.Context) {
	now := s.now()
	statuses, err := s.ListCalendarStatus(ctx)
	if err != nil {
		s.logger.Warn("calendar refresh scan failed", "error", err)
		return
	}
	for _, status := range statuses {
		if !status.Enabled {
			s.logger.Debug("calendar refresh skipped disabled calendar", "calendar_id", status.ID, "key", status.Key, "name", status.Name)
			continue
		}
		if status.NextRefresh == nil || !status.NextRefresh.After(now) {
			_ = s.RefreshCalendar(ctx, status.ID, now)
		} else {
			s.logger.Debug("calendar refresh skipped until next interval", "calendar_id", status.ID, "key", status.Key, "name", status.Name, "next_refresh", status.NextRefresh.Format(time.RFC3339))
		}
	}
}

// RefreshAllCalendars refreshes every enabled calendar and returns a per-calendar summary.
func (s *Service) RefreshAllCalendars(ctx context.Context) ([]RefreshCalendarResult, error) {
	statuses, err := s.ListCalendarStatus(ctx)
	if err != nil {
		return nil, err
	}
	results := make([]RefreshCalendarResult, 0, len(statuses))
	now := s.now()
	for _, status := range statuses {
		if !status.Enabled {
			results = append(results, RefreshCalendarResult{CalendarID: status.ID, CalendarName: status.Name, OK: true, Skipped: true})
			continue
		}
		result := RefreshCalendarResult{CalendarID: status.ID, CalendarName: status.Name}
		if err := s.RefreshCalendar(ctx, status.ID, now); err != nil {
			result.Error = err.Error()
		} else {
			result.OK = true
		}
		results = append(results, result)
	}
	return results, nil
}

// RunRefresher refreshes due calendars until the context is canceled.
func (s *Service) RunRefresher(ctx context.Context) {
	s.logger.Info("calendar refresher started", "refresh_interval", s.refreshInterval.String())
	s.RefreshDueCalendars(ctx)
	ticker := time.NewTicker(s.refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.logger.Info("calendar refresher stopped", "error", ctx.Err())
			return
		case <-ticker.C:
			s.RefreshDueCalendars(ctx)
		}
	}
}

// UpcomingMeetings returns ongoing and future meetings sorted by start time.
func (s *Service) UpcomingMeetings(ctx context.Context, query UpcomingQuery) ([]Meeting, error) {
	now, lookaheadDays := s.resolveUpcomingWindow(query)
	limit := query.limit(10)
	location, timezone, err := s.queryLocation(query)
	if err != nil {
		return nil, err
	}
	query, until := s.applyQueryWindow(query, now, lookaheadDays, location)
	events, err := s.store.queryEvents(ctx, now, until, query.CalendarIDs, 10000, true)
	if err != nil {
		return nil, err
	}
	meetings := filterMeetings(s.meetingsFromEvents(events, now, query, location, timezone), query)
	sortMeetings(meetings, query.Sort)
	if len(meetings) > limit {
		meetings = meetings[:limit]
	}
	return meetings, nil
}

// NextMeeting returns the next meeting-focused event.
func (s *Service) NextMeeting(ctx context.Context, query UpcomingQuery) ([]Meeting, error) {
	query.Limit = 1
	query.ExcludeAllDay = true
	query.ExcludeCancelled = true
	return s.UpcomingMeetings(ctx, query)
}

// TodayMeetings returns meetings for the current day in the requested display timezone.
func (s *Service) TodayMeetings(ctx context.Context, query UpcomingQuery) ([]Meeting, error) {
	location, _, err := s.queryLocation(query)
	if err != nil {
		return nil, err
	}
	now := query.Now
	if now.IsZero() {
		now = s.now()
	}
	localNow := now.In(location)
	localStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	query.Now = now
	query.LookaheadDays = 1
	query.After = localStart.UTC()
	query.Before = localStart.Add(24 * time.Hour).UTC()
	query.ExcludeCancelled = true
	query.OverlapWindow = true
	if query.Sort == "" {
		query.Sort = "agenda"
	}
	return s.UpcomingMeetings(ctx, query)
}

// FreeBusy returns busy blocks without meeting titles or descriptions.
func (s *Service) FreeBusy(ctx context.Context, query UpcomingQuery) ([]BusyBlock, error) {
	query.IncludeDescription = false
	meetings, err := s.UpcomingMeetings(ctx, query)
	if err != nil {
		return nil, err
	}
	busy := make([]BusyBlock, 0, len(meetings))
	for _, meeting := range meetings {
		busy = append(busy, BusyBlock{
			When:            meeting.When,
			Calendar:        meeting.Calendar,
			Duration:        meeting.Duration,
			DurationMinutes: meeting.DurationMinutes,
			Ongoing:         meeting.Ongoing,
			AllDay:          meeting.AllDay,
		})
	}
	return busy, nil
}

// UpcomingMeetingsByCalendar returns upcoming meetings grouped by calendar.
func (s *Service) UpcomingMeetingsByCalendar(ctx context.Context, query UpcomingQuery) ([]CalendarMeetingGroup, error) {
	now, lookaheadDays := s.resolveUpcomingWindow(query)
	location, timezone, err := s.queryLocation(query)
	if err != nil {
		return nil, err
	}
	query, until := s.applyQueryWindow(query, now, lookaheadDays, location)
	events, err := s.store.queryEvents(ctx, now, until, query.CalendarIDs, 10000, true)
	if err != nil {
		return nil, err
	}
	meetings := filterMeetings(s.meetingsFromEvents(events, now, query, location, timezone), query)
	sortMeetings(meetings, query.Sort)
	limitPerCalendar := query.limit(10)
	groupIndex := map[string]int{}
	groups := []CalendarMeetingGroup{}
	for _, meeting := range meetings {
		index, ok := groupIndex[meeting.CalendarID]
		if !ok {
			index = len(groups)
			groupIndex[meeting.CalendarID] = index
			groups = append(groups, CalendarMeetingGroup{
				Calendar:     meeting.CalendarName,
				CalendarID:   meeting.CalendarID,
				CalendarName: meeting.CalendarName,
			})
		}
		if len(groups[index].Meetings) >= limitPerCalendar {
			continue
		}
		groups[index].Meetings = append(groups[index].Meetings, meeting)
	}
	slices.SortFunc(groups, func(a, b CalendarMeetingGroup) int {
		return strings.Compare(a.CalendarName, b.CalendarName)
	})
	return groups, nil
}

func (s *Service) resolveUpcomingWindow(query UpcomingQuery) (time.Time, int) {
	now := query.Now
	if now.IsZero() {
		now = s.now()
	}
	lookaheadDays := query.LookaheadDays
	if lookaheadDays <= 0 {
		lookaheadDays = 30
	}
	return now, lookaheadDays
}

func (s *Service) queryLocation(query UpcomingQuery) (*time.Location, string, error) {
	timezone := strings.TrimSpace(query.Timezone)
	if timezone == "" {
		return s.location, s.timezone, nil
	}
	location, resolvedTimezone, err := loadLocation(timezone)
	if err != nil {
		return nil, "", fmt.Errorf("load timezone %q: %w", timezone, err)
	}
	return location, resolvedTimezone, nil
}

func (s *Service) applyQueryWindow(query UpcomingQuery, now time.Time, lookaheadDays int, location *time.Location) (UpcomingQuery, time.Time) {
	until := now.Add(time.Duration(lookaheadDays) * 24 * time.Hour)
	localNow := now.In(location)
	startOfToday := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	setWindow := func(after, before time.Time) {
		query.After = after.UTC()
		query.Before = before.UTC()
		query.OverlapWindow = true
		until = query.Before
	}
	switch strings.ToLower(strings.TrimSpace(query.Window)) {
	case "today":
		setWindow(startOfToday, startOfToday.Add(24*time.Hour))
	case "tomorrow":
		tomorrow := startOfToday.Add(24 * time.Hour)
		setWindow(tomorrow, tomorrow.Add(24*time.Hour))
	case "today_tomorrow", "today_and_tomorrow":
		setWindow(startOfToday, startOfToday.Add(48*time.Hour))
	case "next_24h", "next_24_hours":
		setWindow(localNow, localNow.Add(24*time.Hour))
	case "workday":
		setWindow(startOfToday.Add(9*time.Hour), startOfToday.Add(17*time.Hour))
	case "rest_of_workday":
		setWindow(localNow, startOfToday.Add(17*time.Hour))
	case "this_week", "rest_of_week":
		setWindow(localNow, startOfToday.Add(time.Duration(daysUntilNextMonday(localNow))*24*time.Hour))
	case "rest_of_work_week":
		workWeekEnd := startOfToday.Add(time.Duration(daysUntilWeekday(localNow, time.Friday))*24*time.Hour + 17*time.Hour)
		setWindow(localNow, workWeekEnd)
	}
	if !query.Before.IsZero() {
		until = query.Before
	}
	if until.Before(now) {
		until = now
	}
	return query, until
}

func daysUntilNextMonday(t time.Time) int {
	days := (int(time.Monday) - int(t.Weekday()) + 7) % 7
	if days == 0 {
		return 7
	}
	return days
}

func daysUntilWeekday(t time.Time, weekday time.Weekday) int {
	return (int(weekday) - int(t.Weekday()) + 7) % 7
}

func (s *Service) meetingsFromEvents(events []EventInstance, now time.Time, query UpcomingQuery, location *time.Location, timezone string) []Meeting {
	meetings := make([]Meeting, 0, len(events))
	for _, event := range events {
		ongoing := event.Start.Before(now) && event.End.After(now)
		localStart := event.Start.In(location)
		localEnd := event.End.In(location)
		meetingURL := event.MeetingURL
		meetingURLType := event.MeetingURLType
		if query.IncludeLinks != nil && !*query.IncludeLinks {
			meetingURL = ""
			meetingURLType = ""
		}
		meeting := Meeting{
			Day:             localStart.Format("Mon"),
			Date:            localStart.Format("2006-01-02"),
			EndDate:         localEnd.Format("2006-01-02"),
			Start:           localStart.Format("15:04"),
			End:             localEnd.Format("15:04"),
			Timezone:        timezone,
			DurationMinutes: int(event.End.Sub(event.Start).Minutes()),
			Name:            event.Name,
			Description:     meetingDescription(event.Description, query),
			MeetingURL:      meetingURL,
			MeetingURLType:  meetingURLType,
			CalendarID:      event.CalendarID,
			CalendarName:    event.CalendarName,
			Ongoing:         ongoing,
			AllDay:          event.AllDay,
			Cancelled:       event.Cancelled,
			Recurring:       event.Recurring,
			RecurrenceID:    event.RecurrenceID,
			StartTime:       event.Start,
			EndTime:         event.End,
			Detail:          query.Detail,
		}
		meeting.When = compactWhen(meeting)
		meeting.Title = meeting.Name
		meeting.Calendar = meeting.CalendarName
		meeting.Duration = durationText(meeting.DurationMinutes)
		meetings = append(meetings, meeting)
	}
	return meetings
}

func filterMeetings(meetings []Meeting, query UpcomingQuery) []Meeting {
	if query.Query == "" && !query.InProgressOnly && !query.ExcludeAllDay && !query.ExcludeCancelled && !query.LinksOnly && query.After.IsZero() && query.Before.IsZero() {
		return meetings
	}
	search := strings.ToLower(strings.TrimSpace(query.Query))
	filtered := make([]Meeting, 0, len(meetings))
	for _, meeting := range meetings {
		if search != "" && !strings.Contains(strings.ToLower(meeting.Name+" "+meeting.Description+" "+meeting.CalendarName), search) {
			continue
		}
		if query.InProgressOnly && !meeting.Ongoing {
			continue
		}
		if query.ExcludeAllDay && meeting.AllDay {
			continue
		}
		if query.ExcludeCancelled && meeting.Cancelled {
			continue
		}
		if query.LinksOnly && meeting.MeetingURL == "" {
			continue
		}
		if !query.After.IsZero() {
			if query.OverlapWindow {
				if !meeting.EndTime.After(query.After) {
					continue
				}
			} else if meeting.StartTime.Before(query.After) {
				continue
			}
		}
		if !query.Before.IsZero() && !meeting.StartTime.Before(query.Before) {
			continue
		}
		filtered = append(filtered, meeting)
	}
	return filtered
}

func sortMeetings(meetings []Meeting, sortMode string) {
	switch strings.ToLower(strings.TrimSpace(sortMode)) {
	case "agenda":
		slices.SortFunc(meetings, compareAgendaMeetings)
	case "calendar":
		slices.SortFunc(meetings, compareCalendarMeetings)
	case "ongoing_first":
		slices.SortFunc(meetings, compareOngoingFirstMeetings)
	default:
		slices.SortFunc(meetings, compareMeetingStartTime)
	}
}

func compareAgendaMeetings(a, b Meeting) int {
	aClass := agendaClass(a)
	bClass := agendaClass(b)
	if aClass != bClass {
		return aClass - bClass
	}
	return compareMeetingStartTime(a, b)
}

func agendaClass(meeting Meeting) int {
	if meeting.Ongoing && !meeting.AllDay {
		return 0
	}
	if !meeting.Ongoing && !meeting.AllDay {
		return 1
	}
	return 2
}

func compareCalendarMeetings(a, b Meeting) int {
	if a.AllDay != b.AllDay {
		if a.AllDay {
			return -1
		}
		return 1
	}
	return compareMeetingStartTime(a, b)
}

func compareOngoingFirstMeetings(a, b Meeting) int {
	if a.Ongoing != b.Ongoing {
		if a.Ongoing {
			return -1
		}
		return 1
	}
	return compareMeetingStartTime(a, b)
}

func compareMeetingStartTime(a, b Meeting) int {
	return a.StartTime.Compare(b.StartTime)
}

func meetingDescription(description string, query UpcomingQuery) string {
	if !query.IncludeDescription {
		return ""
	}
	maxChars := query.DescriptionMaxChars
	if maxChars <= 0 {
		maxChars = 300
	}
	runes := []rune(description)
	if len(runes) <= maxChars {
		return description
	}
	return string(runes[:maxChars]) + "..."
}

// Status returns service state.
func (s *Service) Status(ctx context.Context) (Status, error) {
	calendars, err := s.ListCalendarStatus(ctx)
	if err != nil {
		return Status{}, err
	}
	return Status{Now: s.now(), Version: s.buildInfo, Timezone: s.timezone, ExternalURL: s.externalURL, Calendars: calendars}, nil
}

// ValidateCalendar fetches and parses an ICS feed without saving it.
func (s *Service) ValidateCalendar(ctx context.Context, in ValidateCalendarInput) (ValidateCalendarResult, error) {
	if strings.TrimSpace(in.URL) == "" {
		return ValidateCalendarResult{OK: false, Error: "calendar URL is required"}, fmt.Errorf("calendar URL is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(in.URL), nil)
	if err != nil {
		return ValidateCalendarResult{OK: false, Error: err.Error()}, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return ValidateCalendarResult{OK: false, Error: err.Error()}, err
	}
	defer resp.Body.Close()
	result := ValidateCalendarResult{StatusCode: resp.StatusCode}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		err := fmt.Errorf("fetch %s: status %d", in.URL, resp.StatusCode)
		result.Error = err.Error()
		return result, err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	now := s.now()
	lookahead := in.LookaheadDays
	if lookahead <= 0 {
		lookahead = 30
	}
	events, err := ParseICS(string(body), now, time.Duration(lookahead)*24*time.Hour)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	for i := range events {
		events[i].CalendarName = "Preview"
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	meetings := s.meetingsFromEvents(events, now, UpcomingQuery{}, s.location, s.timezone)
	slices.SortFunc(meetings, func(a, b Meeting) int {
		return a.StartTime.Compare(b.StartTime)
	})
	if len(meetings) > limit {
		meetings = meetings[:limit]
	}
	result.OK = true
	result.EventCount = len(events)
	result.Meetings = meetings
	return result, nil
}

// MetricsText returns Prometheus-compatible service gauges.
func (s *Service) MetricsText(ctx context.Context) (string, error) {
	statuses, err := s.ListCalendarStatus(ctx)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("# HELP icsmcp_calendars_total Number of configured calendars.\n")
	out.WriteString("# TYPE icsmcp_calendars_total gauge\n")
	_, _ = fmt.Fprintf(&out, "icsmcp_calendars_total %d\n", len(statuses))
	out.WriteString("# HELP icsmcp_calendar_events Cached event instances by calendar.\n")
	out.WriteString("# TYPE icsmcp_calendar_events gauge\n")
	for _, status := range statuses {
		_, _ = fmt.Fprintf(&out, "icsmcp_calendar_events{calendar_id=%q,calendar_key=%q,calendar_name=%q} %d\n", status.ID, status.Key, status.Name, status.EventCount)
	}
	return out.String(), nil
}

func calendarsFromEnv(env map[string]string) []Calendar {
	calendars := []Calendar{}
	for key, value := range env {
		if !strings.HasPrefix(key, "ICSMCP_CALENDAR_") || value == "" {
			continue
		}
		suffix := strings.TrimPrefix(key, "ICSMCP_CALENDAR_")
		calendars = append(calendars, Calendar{
			ID:                      stableID(suffix),
			Key:                     suffix,
			Name:                    strings.ReplaceAll(suffix, "_", " "),
			URL:                     value,
			Enabled:                 true,
			IncludeInGeneralQueries: true,
		})
	}
	slices.SortFunc(calendars, func(a, b Calendar) int {
		return strings.Compare(a.Key, b.Key)
	})
	return calendars
}

func calendarFromAssignment(value string) (Calendar, error) {
	key, url, ok := strings.Cut(value, "=")
	if !ok || key == "" || url == "" {
		return Calendar{}, fmt.Errorf("calendar must be name=url")
	}
	return calendarFromInput(AddCalendarInput{Key: key, Name: strings.ReplaceAll(key, "_", " "), URL: url})
}

func calendarFromInput(in AddCalendarInput) (Calendar, error) {
	if in.URL == "" {
		return Calendar{}, fmt.Errorf("calendar URL is required")
	}
	key := in.Key
	if key == "" {
		key = in.Name
	}
	key = normalizeKey(key)
	if key == "" {
		return Calendar{}, fmt.Errorf("calendar key or name is required")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = strings.ReplaceAll(key, "_", " ")
	}
	return Calendar{ID: stableID(key), Key: key, Name: name, URL: strings.TrimSpace(in.URL), Enabled: true, IncludeInGeneralQueries: true}, nil
}

func normalizeKey(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToUpper(value)
	re := regexp.MustCompile(`[^A-Z0-9]+`)
	value = re.ReplaceAllString(value, "_")
	return strings.Trim(value, "_")
}

func stableID(key string) string {
	sum := sha1.Sum([]byte(strings.ToUpper(key)))
	return hex.EncodeToString(sum[:])
}

func uniqueCalendarIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// EnvMap returns the current process environment as a map.
func EnvMap() map[string]string {
	env := map[string]string{}
	for _, item := range os.Environ() {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}
