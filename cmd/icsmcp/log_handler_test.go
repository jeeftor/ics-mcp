package icsmcp

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestColorSlogHandlerColorsLevelMessageKeysAndValues(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(newColorSlogHandler(&out, slog.LevelInfo))

	logger.Info("server starting", "http_addr", "127.0.0.1:3333")

	got := out.String()
	if count := strings.Count(got, "\x1b["); count < 4 {
		t.Fatalf("log output has %d ANSI color escapes, want at least 4:\n%s", count, got)
	}
	stripped := stripANSI(got)
	for _, want := range []string{"INFO", `msg="server starting"`, "http_addr=127.0.0.1:3333"} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("log output missing %q:\n%s", want, stripped)
		}
	}
}

func TestColorSlogHandlerCanDisableColor(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(newPlainSlogHandler(&out, slog.LevelInfo))

	logger.Info("server starting", "http_addr", "127.0.0.1:3333")

	got := out.String()
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("log output contains ANSI color escape:\n%s", got)
	}
	for _, want := range []string{"INFO", `msg="server starting"`, "http_addr=127.0.0.1:3333"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output missing %q:\n%s", want, got)
		}
	}
}

func TestColorSlogHandlerFiltersBelowConfiguredLevel(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(newColorSlogHandler(&out, slog.LevelWarn))

	logger.Info("hidden")
	logger.Warn("visible")

	got := out.String()
	if strings.Contains(got, "hidden") {
		t.Fatalf("log output included filtered info record:\n%s", got)
	}
	if !strings.Contains(got, "visible") {
		t.Fatalf("log output missing warning record:\n%s", got)
	}
}

func TestColorSlogHandlerReturnsWriterErrors(t *testing.T) {
	handler := newPlainSlogHandler(&errWriter{}, slog.LevelInfo)
	record := slog.NewRecord(timeNowForLogTest(), slog.LevelInfo, "write fails", 0)

	err := handler.Handle(context.Background(), record)
	if err == nil || !strings.Contains(err.Error(), io.ErrClosedPipe.Error()) {
		t.Fatalf("Handle() error = %v, want closed pipe", err)
	}
}

func TestColorSlogHandlerFillsZeroRecordTimestamp(t *testing.T) {
	var out bytes.Buffer
	handler := newPlainSlogHandler(&out, slog.LevelInfo)
	record := slog.NewRecord(time.Time{}, slog.LevelInfo, "zero timestamp", 0)

	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `msg="zero timestamp"`) || strings.Contains(got, "time=0001-01-01") {
		t.Fatalf("zero timestamp log output = %q", got)
	}
}

func stripANSI(value string) string {
	var out strings.Builder
	inEscape := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if inEscape {
			if ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' {
				inEscape = false
			}
			continue
		}
		if ch == 0x1b {
			inEscape = true
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func timeNowForLogTest() time.Time {
	return time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
}
