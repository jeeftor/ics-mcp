package icsmcp

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	app "github.com/jeeftor/icsmcp/internal/icsmcp"
)

func TestCalendarFlagsCollectRepeatableValues(t *testing.T) {
	var flags calendarFlags
	if got := flags.Type(); got != "name=url" {
		t.Fatalf("Type() = %q, want name=url", got)
	}
	if err := flags.Set("WORK=https://example.test/work.ics"); err != nil {
		t.Fatalf("Set(first) error = %v", err)
	}
	if err := flags.Set("HOME=https://example.test/home.ics"); err != nil {
		t.Fatalf("Set(second) error = %v", err)
	}
	if got := flags.String(); !strings.Contains(got, "WORK=https://example.test/work.ics") || !strings.Contains(got, "HOME=https://example.test/home.ics") {
		t.Fatalf("String() = %q, want both assignments", got)
	}
}

func TestPrintStartupInfoIncludesAdminAndMCPURLs(t *testing.T) {
	var out strings.Builder

	printStartupInfo(&out, "127.0.0.1:3333", "America/Denver", "https://ics-mcp.example.net")

	got := out.String()
	for _, want := range []string{
		"ICS MCP server listening on 127.0.0.1:3333",
		"Display timezone: America/Denver",
		"Admin UI: http://127.0.0.1:3333/",
		"MCP endpoint: http://127.0.0.1:3333/mcp",
		"Status API: http://127.0.0.1:3333/api/status",
		"External URL: https://ics-mcp.example.net",
		"External MCP endpoint: https://ics-mcp.example.net/mcp",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("startup output missing %q:\n%s", want, got)
		}
	}
}

func TestPrintStartupInfoOmitsExternalURLWhenUnset(t *testing.T) {
	var out strings.Builder

	printStartupInfo(&out, "0.0.0.0:3333", "UTC", "")

	got := out.String()
	if strings.Contains(got, "External URL") || strings.Contains(got, "External MCP endpoint") {
		t.Fatalf("startup output included external URL unexpectedly:\n%s", got)
	}
}

func TestPrintStartupInfoStopsAfterInitialWriteError(t *testing.T) {
	writer := &errWriter{}

	printStartupInfo(writer, "127.0.0.1:3333", "UTC", "https://ics-mcp.example.net")

	if writer.writes != 1 {
		t.Fatalf("write count = %d, want exactly one failed write", writer.writes)
	}
}

func TestVersionCommandPrintsBuildMetadata(t *testing.T) {
	oldVersion, oldCommit, oldDate := Version, Commit, Date
	t.Cleanup(func() {
		Version, Commit, Date = oldVersion, oldCommit, oldDate
	})
	Version = "v1.2.3"
	Commit = "abc123"
	Date = "2026-06-29"

	cmd := NewRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(version) error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"version: v1.2.3", "commit: abc123", "date: 2026-06-29"} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output missing %q:\n%s", want, got)
		}
	}
}

func TestServeCommandRejectsInvalidLogLevelBeforeStarting(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"serve", "--log-level", "verbose"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `invalid log level "verbose"`) {
		t.Fatalf("Execute(serve invalid log-level) error = %v, want invalid log level", err)
	}
}

func TestServeCommandTimezoneHelpMentionsOnlyAppTimezoneConfig(t *testing.T) {
	cmd := NewRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"serve", "--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(serve --help) error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "defaults to ICSMCP_TIMEZONE or UTC") {
		t.Fatalf("serve help missing app timezone default:\n%s", got)
	}
	if strings.Contains(got, "TZ") {
		t.Fatalf("serve help still mentions generic TZ:\n%s", got)
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

func TestResolveDBPathFallsBackToCurrentDirectoryWhenConfigDirEmpty(t *testing.T) {
	got := resolveDBPath("", "")

	if got != "icsmcp.sqlite3" {
		t.Fatalf("resolveDBPath() = %q, want icsmcp.sqlite3", got)
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

func TestRunServeCreatesDatabaseDirAndReturnsStartupImportError(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nested", "icsmcp.sqlite3")
	logger := slog.New(newPlainSlogHandler(io.Discard, slog.LevelError))

	err := runServe(context.Background(), "127.0.0.1:0", dbPath, time.Minute, []string{"missing-separator"}, logger, appBuildInfo(), "UTC", "")
	if err == nil || !strings.Contains(err.Error(), "calendar must be name=url") {
		t.Fatalf("runServe() error = %v, want startup calendar import error", err)
	}
	if _, err := os.Stat(filepath.Dir(dbPath)); err != nil {
		t.Fatalf("database directory was not created: %v", err)
	}
}

func TestRunServeReportsDatabaseDirectoryCreationErrors(t *testing.T) {
	dir := t.TempDir()
	parentFile := filepath.Join(dir, "not-a-directory")
	if err := os.WriteFile(parentFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("write parent fixture: %v", err)
	}
	dbPath := filepath.Join(parentFile, "icsmcp.sqlite3")
	logger := slog.New(newPlainSlogHandler(io.Discard, slog.LevelError))

	err := runServe(context.Background(), "127.0.0.1:0", dbPath, time.Minute, nil, logger, appBuildInfo(), "UTC", "")
	if err == nil || !strings.Contains(err.Error(), "create database directory") {
		t.Fatalf("runServe() error = %v, want database directory context", err)
	}
}

func TestRunServeReportsOpenStoreErrors(t *testing.T) {
	dbPath := t.TempDir()
	logger := slog.New(newPlainSlogHandler(io.Discard, slog.LevelError))

	err := runServe(context.Background(), "127.0.0.1:0", dbPath, time.Minute, nil, logger, appBuildInfo(), "UTC", "")
	if err == nil || !strings.Contains(err.Error(), "migrate sqlite") {
		t.Fatalf("runServe() error = %v, want sqlite migration context", err)
	}
}

func TestRunServeExitsCleanlyWhenContextIsCancelled(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "nested", "icsmcp.sqlite3")
	var logs bytes.Buffer
	logger := slog.New(newPlainSlogHandler(&logs, slog.LevelInfo))
	buildInfo := app.BuildInfo{Version: "v9.9.9", Commit: "abc123", Date: "2026-06-30"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)

	go func() {
		errCh <- runServe(ctx, "127.0.0.1:0", dbPath, time.Minute, nil, logger, buildInfo, "UTC", "")
	}()

	timer := time.NewTimer(50 * time.Millisecond)
	select {
	case err := <-errCh:
		timer.Stop()
		t.Fatalf("runServe() returned before cancellation: %v", err)
	case <-timer.C:
		cancel()
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runServe() error = %v, want clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("runServe() did not exit after context cancellation")
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database was not created: %v", err)
	}
	gotLogs := logs.String()
	for _, want := range []string{"msg=\"server starting\"", "version=v9.9.9", "commit=abc123", "build_date=2026-06-30"} {
		if !strings.Contains(gotLogs, want) {
			t.Fatalf("startup log missing %q:\n%s", want, gotLogs)
		}
	}
}

func TestPlainSlogHandlerIncludesAttrsGroupsAndFormattedValues(t *testing.T) {
	var out bytes.Buffer
	handler := newPlainSlogHandler(&out, slog.LevelDebug).
		WithAttrs([]slog.Attr{slog.String("component", "calendar worker")}).
		WithGroup("request")
	logger := slog.New(handler)
	when := time.Date(2026, 6, 30, 4, 0, 0, 0, time.UTC)

	logger.LogAttrs(context.Background(), slog.LevelWarn, "refresh failed",
		slog.String("calendar_id", "abc123"),
		slog.Time("at", when),
		slog.Duration("retry_in", 5*time.Minute),
		slog.Group("http", slog.Int("status", httpStatusBadGateway)),
	)

	got := out.String()
	for _, want := range []string{
		`level=WARN`,
		`msg="refresh failed"`,
		`component="calendar worker"`,
		`request.calendar_id=abc123`,
		`request.at=2026-06-30T04:00:00Z`,
		`request.retry_in=5m0s`,
		`request.http={status=502}`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output missing %q:\n%s", want, got)
		}
	}
}

func TestPlainSlogHandlerWithEmptyGroupAndEmptyAttrs(t *testing.T) {
	var out bytes.Buffer
	handler := newPlainSlogHandler(&out, slog.LevelInfo).WithGroup("").WithAttrs(nil)
	logger := slog.New(handler)

	logger.Info("ready", slog.String("empty", ""), slog.Bool("ok", true))

	got := out.String()
	for _, want := range []string{`msg="ready"`, `empty=""`, `ok=true`} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output missing %q:\n%s", want, got)
		}
	}
}

func TestColorSlogHandlerRendersAllLevelLabels(t *testing.T) {
	var out bytes.Buffer
	handler := newColorSlogHandler(&out, slog.LevelDebug)
	logger := slog.New(handler)

	logger.Debug("debugging")
	logger.Info("ready")
	logger.Warn("slow")
	logger.Error("failed")

	got := out.String()
	for _, want := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		if !strings.Contains(got, want) {
			t.Fatalf("color log output missing %q:\n%s", want, got)
		}
	}
}

func TestSlogHandlerSkipsEmptyAttrsAndClonesSlices(t *testing.T) {
	handler := newPlainSlogHandler(io.Discard, slog.LevelInfo).(*colorSlogHandler)
	var b strings.Builder
	handler.writeAttr(&b, slog.Attr{})
	if b.Len() != 0 {
		t.Fatalf("empty attr wrote %q, want no output", b.String())
	}

	values := []string{"first"}
	cloned := slicesClone(values)
	values[0] = "changed"
	if len(cloned) != 1 || cloned[0] != "first" {
		t.Fatalf("slicesClone() = %#v after source mutation, want independent copy", cloned)
	}

	var out bytes.Buffer
	logger := slog.New(newPlainSlogHandler(&out, slog.LevelInfo).WithAttrs([]slog.Attr{slog.String("first", "1")}).WithAttrs([]slog.Attr{slog.String("second", "2")}))
	logger.Info("attrs")
	got := out.String()
	for _, want := range []string{"first=1", "second=2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("log output missing %q:\n%s", want, got)
		}
	}
}

const httpStatusBadGateway = 502

type errWriter struct {
	writes int
}

func (w *errWriter) Write([]byte) (int, error) {
	w.writes++
	return 0, io.ErrClosedPipe
}

func appBuildInfo() app.BuildInfo {
	return app.BuildInfo{Version: "test", Commit: "test", Date: "test"}
}
