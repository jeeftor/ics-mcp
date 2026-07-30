package icsmcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store owns SQLite persistence.
type Store struct {
	db *sql.DB
}

// OpenStore opens and migrates a SQLite database.
func OpenStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

// Close releases the database handle.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode = WAL`,
		`CREATE TABLE IF NOT EXISTS calendars (
			id TEXT PRIMARY KEY,
			key TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			color TEXT NOT NULL DEFAULT '',
			icon TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			include_in_general_queries INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`ALTER TABLE calendars ADD COLUMN include_in_general_queries INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE calendars ADD COLUMN color TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE calendars ADD COLUMN icon TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS tags (
			name TEXT PRIMARY KEY,
			normalized_name TEXT NOT NULL UNIQUE
		)`,
		`CREATE TABLE IF NOT EXISTS calendar_tags (
			calendar_id TEXT NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
			tag_name TEXT NOT NULL REFERENCES tags(name) ON DELETE CASCADE,
			PRIMARY KEY (calendar_id, tag_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_calendar_tags_tag_name ON calendar_tags(tag_name)`,
		`CREATE TABLE IF NOT EXISTS runtime_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS refresh_state (
			calendar_id TEXT PRIMARY KEY REFERENCES calendars(id) ON DELETE CASCADE,
			last_attempt TEXT,
			last_success TEXT,
			last_error TEXT NOT NULL DEFAULT '',
			next_refresh TEXT,
			etag TEXT NOT NULL DEFAULT '',
			last_modified TEXT NOT NULL DEFAULT '',
			event_count INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			calendar_id TEXT NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
			uid TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT NOT NULL,
			meeting_url TEXT NOT NULL DEFAULT '',
			meeting_url_type TEXT NOT NULL DEFAULT '',
			cancelled INTEGER NOT NULL DEFAULT 0,
			all_day INTEGER NOT NULL DEFAULT 0,
			recurring INTEGER NOT NULL DEFAULT 0,
			recurrence_id TEXT NOT NULL DEFAULT '',
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL
		)`,
		`ALTER TABLE events ADD COLUMN meeting_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN meeting_url_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN cancelled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE events ADD COLUMN all_day INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE events ADD COLUMN recurring INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE events ADD COLUMN recurrence_id TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_events_start ON events(start_time)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			if strings.Contains(err.Error(), "duplicate column name") {
				continue
			}
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	return nil
}

func (s *Store) upsertCalendar(ctx context.Context, cal Calendar, preserveName bool) (Calendar, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	existing, err := s.calendarByKey(ctx, cal.Key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Calendar{}, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		_, err = s.db.ExecContext(ctx, `INSERT INTO calendars (id, key, name, url, color, icon, enabled, include_in_general_queries, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			cal.ID, cal.Key, cal.Name, cal.URL, cal.Color, cal.Icon, boolInt(cal.Enabled), boolInt(cal.IncludeInGeneralQueries), now, now)
		if err != nil {
			return Calendar{}, fmt.Errorf("insert calendar: %w", err)
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO refresh_state (calendar_id) VALUES (?)`, cal.ID)
		if err != nil {
			return Calendar{}, fmt.Errorf("insert refresh state: %w", err)
		}
		if err := s.replaceCalendarTags(ctx, cal.ID, cal.Tags); err != nil {
			return Calendar{}, err
		}
		return s.calendarByID(ctx, cal.ID)
	}

	name := cal.Name
	if preserveName {
		name = existing.Name
	}
	enabled := cal.Enabled
	if !enabled {
		enabled = existing.Enabled
	}
	color, icon := cal.Color, cal.Icon
	if preserveName {
		color, icon = existing.Color, existing.Icon
	}
	_, err = s.db.ExecContext(ctx, `UPDATE calendars SET name = ?, url = ?, color = ?, icon = ?, enabled = ?, include_in_general_queries = ?, updated_at = ? WHERE id = ?`,
		name, cal.URL, color, icon, boolInt(enabled), boolInt(existing.IncludeInGeneralQueries), now, existing.ID)
	if err != nil {
		return Calendar{}, fmt.Errorf("update calendar: %w", err)
	}
	if cal.Tags != nil && !preserveName {
		if err := s.replaceCalendarTags(ctx, existing.ID, cal.Tags); err != nil {
			return Calendar{}, err
		}
	}
	return s.calendarByID(ctx, existing.ID)
}

func (s *Store) calendarByKey(ctx context.Context, key string) (Calendar, error) {
	cal, err := scanCalendar(s.db.QueryRowContext(ctx, `SELECT id, key, name, url, color, icon, enabled, include_in_general_queries FROM calendars WHERE key = ?`, key))
	if err != nil {
		return Calendar{}, err
	}
	return s.withCalendarTags(ctx, cal)
}

func (s *Store) calendarByID(ctx context.Context, id string) (Calendar, error) {
	cal, err := scanCalendar(s.db.QueryRowContext(ctx, `SELECT id, key, name, url, color, icon, enabled, include_in_general_queries FROM calendars WHERE id = ?`, id))
	if err != nil {
		return Calendar{}, err
	}
	return s.withCalendarTags(ctx, cal)
}

func (s *Store) listCalendars(ctx context.Context) ([]Calendar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, key, name, url, color, icon, enabled, include_in_general_queries FROM calendars ORDER BY name, key`)
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}
	defer rows.Close()
	calendars := []Calendar{}
	for rows.Next() {
		cal, err := scanCalendar(rows)
		if err != nil {
			return nil, fmt.Errorf("scan calendar: %w", err)
		}
		cal, err = s.withCalendarTags(ctx, cal)
		if err != nil {
			return nil, err
		}
		calendars = append(calendars, cal)
	}
	return calendars, rows.Err()
}

func (s *Store) updateCalendar(ctx context.Context, id string, in UpdateCalendarInput) (Calendar, error) {
	cal, err := s.calendarByID(ctx, id)
	if err != nil {
		return Calendar{}, err
	}
	if in.Name != "" {
		cal.Name = in.Name
	}
	if in.URL != "" {
		cal.URL = in.URL
	}
	if in.Enabled != nil {
		cal.Enabled = *in.Enabled
	}
	if in.IncludeInGeneralQueries != nil {
		cal.IncludeInGeneralQueries = *in.IncludeInGeneralQueries
	}
	if in.Color != "" {
		cal.Color = in.Color
	}
	if in.Icon != "" {
		cal.Icon = in.Icon
	}
	_, err = s.db.ExecContext(ctx, `UPDATE calendars SET name = ?, url = ?, color = ?, icon = ?, enabled = ?, include_in_general_queries = ?, updated_at = ? WHERE id = ?`,
		cal.Name, cal.URL, cal.Color, cal.Icon, boolInt(cal.Enabled), boolInt(cal.IncludeInGeneralQueries), time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return Calendar{}, fmt.Errorf("update calendar: %w", err)
	}
	if in.Tags != nil {
		if err := s.replaceCalendarTags(ctx, id, *in.Tags); err != nil {
			return Calendar{}, err
		}
	}
	return s.calendarByID(ctx, id)
}

func (s *Store) setGeneralQueryCalendarIDs(ctx context.Context, calendarIDs []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin calendar selection update: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT id FROM calendars`)
	if err != nil {
		return fmt.Errorf("list calendar ids: %w", err)
	}
	known := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan calendar id: %w", err)
		}
		known[id] = true
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close calendar id rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate calendar ids: %w", err)
	}

	for _, id := range calendarIDs {
		if !known[id] {
			return fmt.Errorf("unknown calendar id %q", id)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE calendars SET include_in_general_queries = 0, updated_at = ?`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("clear calendar selection: %w", err)
	}
	if len(calendarIDs) > 0 {
		args := make([]any, 0, len(calendarIDs)+1)
		args = append(args, time.Now().UTC().Format(time.RFC3339Nano))
		for _, id := range calendarIDs {
			args = append(args, id)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE calendars SET include_in_general_queries = 1, updated_at = ? WHERE id IN (`+placeholders(len(calendarIDs))+`)`, args...); err != nil {
			return fmt.Errorf("save calendar selection: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit calendar selection update: %w", err)
	}
	return nil
}

func (s *Store) deleteCalendar(ctx context.Context, id string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE calendar_id = ?`, id); err != nil {
		return fmt.Errorf("delete events: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM refresh_state WHERE calendar_id = ?`, id); err != nil {
		return fmt.Errorf("delete refresh state: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM calendars WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete calendar: %w", err)
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM tags WHERE NOT EXISTS (SELECT 1 FROM calendar_tags WHERE calendar_tags.tag_name = tags.name)`)
	if err != nil {
		return fmt.Errorf("delete unused tags: %w", err)
	}
	return nil
}

func (s *Store) replaceEvents(ctx context.Context, calendarID string, events []EventInstance) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace events: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE calendar_id = ?`, calendarID); err != nil {
		return fmt.Errorf("clear events: %w", err)
	}
	for _, event := range events {
		if _, err := tx.ExecContext(ctx, `INSERT INTO events (id, calendar_id, uid, name, description, meeting_url, meeting_url_type, cancelled, all_day, recurring, recurrence_id, start_time, end_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			event.ID, calendarID, event.UID, event.Name, event.Description, event.MeetingURL, event.MeetingURLType, boolInt(event.Cancelled), boolInt(event.AllDay), boolInt(event.Recurring), event.RecurrenceID, event.Start.UTC().Format(time.RFC3339Nano), event.End.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace events: %w", err)
	}
	return nil
}

func (s *Store) queryEvents(ctx context.Context, now, until time.Time, calendarIDs []string, limit int, generalOnly bool, includeDisabled bool) ([]EventInstance, error) {
	query := `SELECT e.id, e.calendar_id, c.name, e.uid, e.name, e.description, e.meeting_url, e.meeting_url_type, e.cancelled, e.all_day, e.recurring, e.recurrence_id, e.start_time, e.end_time
		FROM events e JOIN calendars c ON c.id = e.calendar_id
		WHERE e.end_time > ? AND e.start_time <= ?`
	args := []any{now.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano)}
	if !includeDisabled {
		query += ` AND c.enabled = 1`
	}
	if len(calendarIDs) > 0 {
		query += ` AND e.calendar_id IN (` + placeholders(len(calendarIDs)) + `)`
		for _, id := range calendarIDs {
			args = append(args, id)
		}
	} else if generalOnly {
		query += ` AND c.include_in_general_queries = 1`
	}
	query += ` ORDER BY e.start_time ASC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()
	events := []EventInstance{}
	for rows.Next() {
		var start, end string
		var event EventInstance
		var cancelled, allDay, recurring int
		if err := rows.Scan(&event.ID, &event.CalendarID, &event.CalendarName, &event.UID, &event.Name, &event.Description, &event.MeetingURL, &event.MeetingURLType, &cancelled, &allDay, &recurring, &event.RecurrenceID, &start, &end); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.Cancelled = cancelled == 1
		event.AllDay = allDay == 1
		event.Recurring = recurring == 1
		var err error
		event.Start, err = time.Parse(time.RFC3339Nano, start)
		if err != nil {
			return nil, fmt.Errorf("parse event start: %w", err)
		}
		event.End, err = time.Parse(time.RFC3339Nano, end)
		if err != nil {
			return nil, fmt.Errorf("parse event end: %w", err)
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Store) updateRefreshState(ctx context.Context, calendarID string, state refreshState) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO refresh_state (calendar_id, last_attempt, last_success, last_error, next_refresh, etag, last_modified, event_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(calendar_id) DO UPDATE SET
			last_attempt = excluded.last_attempt,
			last_success = excluded.last_success,
			last_error = excluded.last_error,
			next_refresh = excluded.next_refresh,
			etag = excluded.etag,
			last_modified = excluded.last_modified,
			event_count = excluded.event_count`,
		calendarID, formatTimePtr(state.LastAttempt), formatTimePtr(state.LastSuccess), state.LastError, formatTimePtr(state.NextRefresh), state.ETag, state.LastModified, state.EventCount)
	if err != nil {
		return fmt.Errorf("update refresh state: %w", err)
	}
	return nil
}

func (s *Store) refreshState(ctx context.Context, calendarID string) (refreshState, error) {
	row := s.db.QueryRowContext(ctx, `SELECT last_attempt, last_success, last_error, next_refresh, etag, last_modified, event_count FROM refresh_state WHERE calendar_id = ?`, calendarID)
	var state refreshState
	var lastAttempt, lastSuccess, nextRefresh sql.NullString
	if err := row.Scan(&lastAttempt, &lastSuccess, &state.LastError, &nextRefresh, &state.ETag, &state.LastModified, &state.EventCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return state, nil
		}
		return state, fmt.Errorf("scan refresh state: %w", err)
	}
	state.LastAttempt = parseTimePtr(lastAttempt)
	state.LastSuccess = parseTimePtr(lastSuccess)
	state.NextRefresh = parseTimePtr(nextRefresh)
	return state, nil
}

func (s *Store) listCalendarStatus(ctx context.Context) ([]CalendarStatus, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.key, c.name, c.url, c.color, c.icon, c.enabled, c.include_in_general_queries,
		rs.last_attempt, rs.last_success, rs.last_error, rs.next_refresh, rs.etag, rs.last_modified, rs.event_count
		FROM calendars c LEFT JOIN refresh_state rs ON rs.calendar_id = c.id
		ORDER BY c.name, c.key`)
	if err != nil {
		return nil, fmt.Errorf("list calendar status: %w", err)
	}
	defer rows.Close()
	statuses := []CalendarStatus{}
	for rows.Next() {
		var status CalendarStatus
		var enabled, includeInGeneralQueries int
		var lastAttempt, lastSuccess, nextRefresh sql.NullString
		var lastError, etag, lastModified sql.NullString
		var eventCount sql.NullInt64
		if err := rows.Scan(&status.ID, &status.Key, &status.Name, &status.URL, &status.Color, &status.Icon, &enabled, &includeInGeneralQueries, &lastAttempt, &lastSuccess, &lastError, &nextRefresh, &etag, &lastModified, &eventCount); err != nil {
			return nil, fmt.Errorf("scan calendar status: %w", err)
		}
		status.Enabled = enabled != 0
		status.IncludeInGeneralQueries = includeInGeneralQueries != 0
		status.LastAttempt = parseTimePtr(lastAttempt)
		status.LastSuccess = parseTimePtr(lastSuccess)
		status.LastError = lastError.String
		status.NextRefresh = parseTimePtr(nextRefresh)
		status.ETag = etag.String
		status.LastModified = lastModified.String
		status.EventCount = int(eventCount.Int64)
		status.Calendar, err = s.withCalendarTags(ctx, status.Calendar)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
}

func (s *Store) withCalendarTags(ctx context.Context, cal Calendar) (Calendar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.name FROM tags t JOIN calendar_tags ct ON ct.tag_name = t.name WHERE ct.calendar_id = ? ORDER BY t.normalized_name`, cal.ID)
	if err != nil {
		return Calendar{}, fmt.Errorf("list calendar tags: %w", err)
	}
	defer rows.Close()
	cal.Tags = []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return Calendar{}, fmt.Errorf("scan calendar tag: %w", err)
		}
		cal.Tags = append(cal.Tags, tag)
	}
	if err := rows.Err(); err != nil {
		return Calendar{}, fmt.Errorf("iterate calendar tags: %w", err)
	}
	return cal, nil
}

func (s *Store) replaceCalendarTags(ctx context.Context, calendarID string, values []string) error {
	tags := normalizeTags(values)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin replace calendar tags: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM calendar_tags WHERE calendar_id = ?`, calendarID); err != nil {
		return fmt.Errorf("clear calendar tags: %w", err)
	}
	for _, tag := range tags {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tags (name, normalized_name) VALUES (?, ?) ON CONFLICT(normalized_name) DO NOTHING`, tag, strings.ToLower(tag)); err != nil {
			return fmt.Errorf("insert tag: %w", err)
		}
		var storedName string
		if err := tx.QueryRowContext(ctx, `SELECT name FROM tags WHERE normalized_name = ?`, strings.ToLower(tag)).Scan(&storedName); err != nil {
			return fmt.Errorf("load tag: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO calendar_tags (calendar_id, tag_name) VALUES (?, ?)`, calendarID, storedName); err != nil {
			return fmt.Errorf("attach calendar tag: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tags WHERE NOT EXISTS (SELECT 1 FROM calendar_tags WHERE calendar_tags.tag_name = tags.name)`); err != nil {
		return fmt.Errorf("delete unused tags: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit calendar tags: %w", err)
	}
	return nil
}

func (s *Store) listTags(ctx context.Context) ([]CalendarTag, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT t.name, COUNT(ct.calendar_id) FROM tags t LEFT JOIN calendar_tags ct ON ct.tag_name = t.name GROUP BY t.name, t.normalized_name ORDER BY t.normalized_name`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()
	tags := []CalendarTag{}
	for rows.Next() {
		var tag CalendarTag
		if err := rows.Scan(&tag.Name, &tag.CalendarCount); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *Store) runtimeSetting(ctx context.Context, key string) (string, bool, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM runtime_settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load runtime setting: %w", err)
	}
	return value, true, nil
}

func (s *Store) setRuntimeSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO runtime_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	if err != nil {
		return fmt.Errorf("save runtime setting: %w", err)
	}
	return nil
}

func (s *Store) calendarIDsByTags(ctx context.Context, values []string) ([]string, error) {
	tags := normalizeTags(values)
	if len(tags) == 0 {
		return []string{}, nil
	}
	args := make([]any, 0, len(tags))
	for _, tag := range tags {
		args = append(args, strings.ToLower(tag))
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT ct.calendar_id FROM calendar_tags ct JOIN tags t ON t.name = ct.tag_name WHERE t.normalized_name IN (`+placeholders(len(tags))+`) ORDER BY ct.calendar_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("find calendars by tags: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan tag calendar id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func normalizeTags(values []string) []string {
	byName := map[string]string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := byName[key]; !exists {
			byName[key] = value
		}
	}
	tags := make([]string, 0, len(byName))
	for _, value := range byName {
		tags = append(tags, value)
	}
	slices.SortFunc(tags, func(a, b string) int { return strings.Compare(strings.ToLower(a), strings.ToLower(b)) })
	return tags
}

type refreshState struct {
	LastAttempt  *time.Time
	LastSuccess  *time.Time
	LastError    string
	NextRefresh  *time.Time
	ETag         string
	LastModified string
	EventCount   int
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanCalendar(row rowScanner) (Calendar, error) {
	var cal Calendar
	var enabled, includeInGeneralQueries int
	if err := row.Scan(&cal.ID, &cal.Key, &cal.Name, &cal.URL, &cal.Color, &cal.Icon, &enabled, &includeInGeneralQueries); err != nil {
		return Calendar{}, err
	}
	cal.Enabled = enabled != 0
	cal.IncludeInGeneralQueries = includeInGeneralQueries != 0
	return cal, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func placeholders(count int) string {
	out := "?"
	for i := 1; i < count; i++ {
		out += ",?"
	}
	return out
}

func formatTimePtr(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func parseTimePtr(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	if err != nil {
		return nil
	}
	return &parsed
}
