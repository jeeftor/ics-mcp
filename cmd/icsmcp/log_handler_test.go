package icsmcp

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestColorSlogHandlerColorsLevelAndKeepsAttributes(t *testing.T) {
	var out bytes.Buffer
	logger := slog.New(newColorSlogHandler(&out, slog.LevelInfo))

	logger.Info("server starting", "http_addr", "127.0.0.1:3333")

	got := out.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("log output does not contain ANSI color escape:\n%s", got)
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
