package icsmcp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
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
			start_time TEXT NOT NULL,
			end_time TEXT NOT NULL
		)`,
		`ALTER TABLE events ADD COLUMN meeting_url TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN meeting_url_type TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN cancelled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE events ADD COLUMN all_day INTEGER NOT NULL DEFAULT 0`,
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
		_, err = s.db.ExecContext(ctx, `INSERT INTO calendars (id, key, name, url, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			cal.ID, cal.Key, cal.Name, cal.URL, boolInt(cal.Enabled), now, now)
		if err != nil {
			return Calendar{}, fmt.Errorf("insert calendar: %w", err)
		}
		_, err = s.db.ExecContext(ctx, `INSERT INTO refresh_state (calendar_id) VALUES (?)`, cal.ID)
		if err != nil {
			return Calendar{}, fmt.Errorf("insert refresh state: %w", err)
		}
		return cal, nil
	}

	name := cal.Name
	if preserveName {
		name = existing.Name
	}
	enabled := cal.Enabled
	if !enabled {
		enabled = existing.Enabled
	}
	_, err = s.db.ExecContext(ctx, `UPDATE calendars SET name = ?, url = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		name, cal.URL, boolInt(enabled), now, existing.ID)
	if err != nil {
		return Calendar{}, fmt.Errorf("update calendar: %w", err)
	}
	existing.Name = name
	existing.URL = cal.URL
	existing.Enabled = enabled
	return existing, nil
}

func (s *Store) calendarByKey(ctx context.Context, key string) (Calendar, error) {
	return scanCalendar(s.db.QueryRowContext(ctx, `SELECT id, key, name, url, enabled FROM calendars WHERE key = ?`, key))
}

func (s *Store) calendarByID(ctx context.Context, id string) (Calendar, error) {
	return scanCalendar(s.db.QueryRowContext(ctx, `SELECT id, key, name, url, enabled FROM calendars WHERE id = ?`, id))
}

func (s *Store) listCalendars(ctx context.Context) ([]Calendar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, key, name, url, enabled FROM calendars ORDER BY name, key`)
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}
	defer rows.Close()
	calendars := []Calendar{}
	for rows.Next() {
		cal, err := scanCalendar(rows)
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
	_, err = s.db.ExecContext(ctx, `UPDATE calendars SET name = ?, url = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		cal.Name, cal.URL, boolInt(cal.Enabled), time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return Calendar{}, fmt.Errorf("update calendar: %w", err)
	}
	return cal, nil
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
		if _, err := tx.ExecContext(ctx, `INSERT INTO events (id, calendar_id, uid, name, description, meeting_url, meeting_url_type, cancelled, all_day, start_time, end_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			event.ID, calendarID, event.UID, event.Name, event.Description, event.MeetingURL, event.MeetingURLType, boolInt(event.Cancelled), boolInt(event.AllDay), event.Start.UTC().Format(time.RFC3339Nano), event.End.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit replace events: %w", err)
	}
	return nil
}

func (s *Store) queryEvents(ctx context.Context, now, until time.Time, calendarIDs []string, limit int) ([]EventInstance, error) {
	query := `SELECT e.id, e.calendar_id, c.name, e.uid, e.name, e.description, e.meeting_url, e.meeting_url_type, e.cancelled, e.all_day, e.start_time, e.end_time
		FROM events e JOIN calendars c ON c.id = e.calendar_id
		WHERE c.enabled = 1 AND e.end_time > ? AND e.start_time <= ?`
	args := []any{now.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano)}
	if len(calendarIDs) > 0 {
		query += ` AND e.calendar_id IN (` + placeholders(len(calendarIDs)) + `)`
		for _, id := range calendarIDs {
			args = append(args, id)
		}
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
		var cancelled, allDay int
		if err := rows.Scan(&event.ID, &event.CalendarID, &event.CalendarName, &event.UID, &event.Name, &event.Description, &event.MeetingURL, &event.MeetingURLType, &cancelled, &allDay, &start, &end); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		event.Cancelled = cancelled == 1
		event.AllDay = allDay == 1
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
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.key, c.name, c.url, c.enabled,
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
		var enabled int
		var lastAttempt, lastSuccess, nextRefresh sql.NullString
		if err := rows.Scan(&status.ID, &status.Key, &status.Name, &status.URL, &enabled, &lastAttempt, &lastSuccess, &status.LastError, &nextRefresh, &status.ETag, &status.LastModified, &status.EventCount); err != nil {
			return nil, fmt.Errorf("scan calendar status: %w", err)
		}
		status.Enabled = enabled != 0
		status.LastAttempt = parseTimePtr(lastAttempt)
		status.LastSuccess = parseTimePtr(lastSuccess)
		status.NextRefresh = parseTimePtr(nextRefresh)
		statuses = append(statuses, status)
	}
	return statuses, rows.Err()
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
	var enabled int
	if err := row.Scan(&cal.ID, &cal.Key, &cal.Name, &cal.URL, &enabled); err != nil {
		return Calendar{}, err
	}
	cal.Enabled = enabled != 0
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
