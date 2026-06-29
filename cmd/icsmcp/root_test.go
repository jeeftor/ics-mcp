package icsmcp

import (
	"log/slog"
	"strings"
	"testing"
)

func TestPrintStartupInfoIncludesAdminAndMCPURLs(t *testing.T) {
	var out strings.Builder

	printStartupInfo(&out, "127.0.0.1:3333")

	got := out.String()
	for _, want := range []string{
		"ICS MCP server listening on 127.0.0.1:3333",
		"Admin UI: http://127.0.0.1:3333/",
		"MCP endpoint: http://127.0.0.1:3333/mcp",
		"Status API: http://127.0.0.1:3333/api/status",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("startup output missing %q:\n%s", want, got)
		}
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"":        slog.LevelInfo,
		"INFO":    slog.LevelInfo,
		"debug":   slog.LevelDebug,
		"Warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"ERROR":   slog.LevelError,
	}

	for input, want := range tests {
		got, err := parseLogLevel(input)
		if err != nil {
			t.Fatalf("parseLogLevel(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("parseLogLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestParseLogLevelRejectsUnknownLevel(t *testing.T) {
	if _, err := parseLogLevel("verbose"); err == nil {
		t.Fatalf("parseLogLevel(verbose) error = nil, want error")
	}
}

func TestResolveDBPathUsesConfigDirByDefault(t *testing.T) {
	got := resolveDBPath("/config", "")

	if got != "/config/icsmcp.sqlite3" {
		t.Fatalf("resolveDBPath() = %q, want /config/icsmcp.sqlite3", got)
	}
}

func TestResolveDBPathAllowsExplicitOverride(t *testing.T) {
	got := resolveDBPath("/config", "/tmp/custom.sqlite3")

	if got != "/tmp/custom.sqlite3" {
		t.Fatalf("resolveDBPath() = %q, want /tmp/custom.sqlite3", got)
	}
}
