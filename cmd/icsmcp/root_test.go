package icsmcp

import (
	"log/slog"
	"os"
	"path/filepath"
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

func TestLoadEnvFilesPrefersConfigDir(t *testing.T) {
	const key = "ICSMCP_CALENDAR_PRECEDENCE"
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
		_ = os.Unsetenv(key)
	})
	_ = os.Unsetenv(key)

	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(key+"=root\n"), 0o600); err != nil {
		t.Fatalf("write root .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, ".env"), []byte(key+"=config\n"), 0o600); err != nil {
		t.Fatalf("write config .env: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}

	loadEnvFiles(configDir)

	if got := os.Getenv(key); got != "config" {
		t.Fatalf("%s = %q, want config", key, got)
	}
}
