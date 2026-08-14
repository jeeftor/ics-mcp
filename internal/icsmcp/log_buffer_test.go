package icsmcp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogBufferRingEviction(t *testing.T) {
	buf := NewLogBuffer(3, slog.LevelInfo)
	for i := range 5 {
		buf.Add(LogEntry{Time: "t", Level: "INFO", Msg: string(rune('A' + i))})
	}
	got := buf.Recent(0)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	// Oldest two (A, B) should have been evicted; C, D, E remain.
	want := []string{"C", "D", "E"}
	for i, e := range got {
		if e.Msg != want[i] {
			t.Errorf("entry %d: got %q, want %q", i, e.Msg, want[i])
		}
	}
}

func TestLogBufferRecentLimit(t *testing.T) {
	buf := NewLogBuffer(10, slog.LevelInfo)
	for i := range 5 {
		buf.Add(LogEntry{Msg: string(rune('A' + i))})
	}
	got := buf.Recent(2)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Msg != "D" || got[1].Msg != "E" {
		t.Errorf("expected D,E, got %s,%s", got[0].Msg, got[1].Msg)
	}
}

func TestSnapshotHandlerCapturesAndFilters(t *testing.T) {
	buf := NewLogBuffer(100, slog.LevelWarn)
	inner := slog.NewTextHandler(io.Discard, nil)
	handler := NewSnapshotHandler(inner, buf)
	logger := slog.New(handler)

	logger.Info("info message", "key", "value")
	logger.Warn("warn message", "calendar", "Work")
	logger.Error("error message", "err", "boom")

	entries := buf.Recent(0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (warn+error), got %d", len(entries))
	}
	if entries[0].Level != "WARN" || entries[0].Msg != "warn message" {
		t.Errorf("entry 0: %+v", entries[0])
	}
	if entries[1].Level != "ERROR" || entries[1].Msg != "error message" {
		t.Errorf("entry 1: %+v", entries[1])
	}
	if entries[0].Attrs["calendar"] != "Work" {
		t.Errorf("expected calendar attr, got %v", entries[0].Attrs)
	}
}

func TestSnapshotHandlerWithAttrs(t *testing.T) {
	buf := NewLogBuffer(100, slog.LevelInfo)
	inner := slog.NewTextHandler(io.Discard, nil)
	handler := NewSnapshotHandler(inner, buf)
	logger := slog.New(handler).With("service", "icsmcp")

	logger.Info("test message", "calendar", "Work")

	entries := buf.Recent(0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Attrs["service"] != "icsmcp" {
		t.Errorf("expected service attr from WithAttrs, got %v", entries[0].Attrs)
	}
	if entries[0].Attrs["calendar"] != "Work" {
		t.Errorf("expected calendar attr, got %v", entries[0].Attrs)
	}
}

func TestRecentLogsNilBuffer(t *testing.T) {
	svc := &Service{logBuffer: nil}
	if entries := svc.RecentLogs(10); entries != nil {
		t.Errorf("expected nil from nil buffer, got %v", entries)
	}
}

func TestAPILogsEndpoint(t *testing.T) {
	buf := NewLogBuffer(100, slog.LevelInfo)
	buf.Add(LogEntry{Time: "2026-01-01T00:00:00Z", Level: "WARN", Msg: "calendar refresh failed", Attrs: map[string]any{"name": "Work", "error": "timeout"}})
	buf.Add(LogEntry{Time: "2026-01-01T00:01:00Z", Level: "INFO", Msg: "calendar refresh succeeded", Attrs: map[string]any{"name": "Personal", "event_count": float64(5)}})

	svc := newTestServiceWithLogBuffer(t, buf)
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var result struct {
		Entries []LogEntry `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result.Entries))
	}
	if result.Entries[0].Msg != "calendar refresh failed" {
		t.Errorf("entry 0: %s", result.Entries[0].Msg)
	}
	if result.Entries[0].Attrs["name"] != "Work" {
		t.Errorf("entry 0 attrs: %v", result.Entries[0].Attrs)
	}
}

func TestAPILogsWithLimit(t *testing.T) {
	buf := NewLogBuffer(100, slog.LevelInfo)
	for i := range 10 {
		buf.Add(LogEntry{Msg: string(rune('A' + i))})
	}
	svc := newTestServiceWithLogBuffer(t, buf)
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Get(server.URL + "/api/logs?limit=3")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var result struct {
		Entries []LogEntry `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(result.Entries))
	}
}

func TestAPILogsMethodNotAllowed(t *testing.T) {
	svc := newTestServiceWithLogBuffer(t, NewLogBuffer(10, slog.LevelInfo))
	server := httptest.NewServer(NewHTTPHandler(svc, NewMCPServer(svc)))
	defer server.Close()

	resp, err := http.Post(server.URL+"/api/logs", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}
}

func newTestServiceWithLogBuffer(t *testing.T, buf *LogBuffer) *Service {
	t.Helper()
	store, err := OpenStore(t.TempDir() + "/icsmcp.sqlite3")
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return NewService(store, ServiceOptions{LogBuffer: buf})
}
