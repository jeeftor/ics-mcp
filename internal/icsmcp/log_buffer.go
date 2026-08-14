package icsmcp

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// LogEntry is a single captured log record suitable for JSON serialization
// to diagnostic endpoints.
type LogEntry struct {
	Time   string         `json:"time"`
	Level  string         `json:"level"`
	Msg    string         `json:"msg"`
	Attrs  map[string]any `json:"attrs,omitempty"`
	Groups []string       `json:"groups,omitempty"`
}

// logsQuery is the MCP tool input for get_logs.
type logsQuery struct {
	Limit int `json:"limit,omitempty"`
}

// logsOutput is the MCP tool output for get_logs.
type logsOutput struct {
	Entries []LogEntry `json:"entries"`
}

// LogBuffer is a bounded ring buffer that retains recent log entries in
// memory so they can be surfaced through the /api/logs REST endpoint and
// the get_logs MCP tool for AI-assisted diagnostics.
type LogBuffer struct {
	mu      sync.Mutex
	entries []LogEntry
	head    int
	count   int
	max     int
	min     slog.Level
}

// NewLogBuffer creates a buffer that holds up to max entries with level >= min.
func NewLogBuffer(max int, min slog.Level) *LogBuffer {
	if max <= 0 {
		max = 200
	}
	return &LogBuffer{
		entries: make([]LogEntry, max),
		max:     max,
		min:     min,
	}
}

// Add appends a log entry to the ring buffer, evicting the oldest entry
// when the buffer is full.
func (b *LogBuffer) Add(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.max
	if b.count < b.max {
		b.count++
	}
}

// Recent returns up to limit entries in chronological order (oldest first).
// If limit <= 0, all buffered entries are returned.
func (b *LogBuffer) Recent(limit int) []LogEntry {
	b.mu.Lock()
	defer b.mu.Unlock()
	if limit <= 0 || limit > b.count {
		limit = b.count
	}
	result := make([]LogEntry, 0, limit)
	start := (b.head - b.count + b.max) % b.max
	for i := 0; i < b.count; i++ {
		idx := (start + i) % b.max
		result = append(result, b.entries[idx])
	}
	if len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result
}

// snapshotHandler is an slog.Handler that wraps an underlying handler and
// copies each record into a LogBuffer before delegating to the wrapped
// handler for formatting and output.
type snapshotHandler struct {
	inner   slog.Handler
	buffer  *LogBuffer
	attrs   []slog.Attr
	groups  []string
}

// NewSnapshotHandler wraps an existing slog.Handler and mirrors records
// at or above the buffer's minimum level into the provided LogBuffer.
func NewSnapshotHandler(inner slog.Handler, buffer *LogBuffer) slog.Handler {
	return &snapshotHandler{inner: inner, buffer: buffer}
}

func (h *snapshotHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *snapshotHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level >= h.buffer.min {
		entry := LogEntry{
			Time:  record.Time.UTC().Format(time.RFC3339),
			Level: record.Level.String(),
			Msg:   record.Message,
			Attrs: map[string]any{},
		}
		for _, attr := range h.attrs {
			entry.Attrs[attr.Key] = attr.Value.Resolve().Any()
		}
		record.Attrs(func(attr slog.Attr) bool {
			resolved := attr.Value.Resolve()
			if resolved.Kind() == slog.KindGroup {
				attrs := resolved.Group()
				groupMap := map[string]any{}
				for _, ga := range attrs {
					groupMap[ga.Key] = ga.Value.Resolve().Any()
				}
				entry.Attrs[attr.Key] = groupMap
			} else {
				entry.Attrs[attr.Key] = resolved.Any()
			}
			return true
		})
		if len(h.groups) > 0 {
			entry.Groups = append([]string(nil), h.groups...)
		}
		h.buffer.Add(entry)
	}
	return h.inner.Handle(ctx, record)
}

func (h *snapshotHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := *h
	next.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	next.inner = h.inner.WithAttrs(attrs)
	return &next
}

func (h *snapshotHandler) WithGroup(name string) slog.Handler {
	next := *h
	if name != "" {
		next.groups = append(append([]string(nil), h.groups...), name)
	}
	next.inner = h.inner.WithGroup(name)
	return &next
}
