package icsmcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	app "github.com/jeeftor/icsmcp/internal/icsmcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

type calendarFlags []string

func (c *calendarFlags) String() string {
	return fmt.Sprint([]string(*c))
}

func (c *calendarFlags) Set(value string) error {
	*c = append(*c, value)
	return nil
}

func (c *calendarFlags) Type() string {
	return "name=url"
}

// NewRootCommand builds the CLI.
func NewRootCommand() *cobra.Command {
	var calendars calendarFlags
	root := &cobra.Command{
		Use:   "icsmcp",
		Short: "ICS Calendar MCP server",
	}
	serve := &cobra.Command{
		Use:   "serve",
		Short: "Run the HTTP admin and MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := viper.GetString("config-dir")
			loadEnvFiles(configDir)
			httpAddr := viper.GetString("http-addr")
			dbPath := resolveDBPath(configDir, viper.GetString("db-path"))
			refreshInterval := viper.GetDuration("refresh-interval")
			logLevel, err := parseLogLevel(viper.GetString("log-level"))
			if err != nil {
				return err
			}
			logger := slog.New(newSlogHandler(os.Stderr, logLevel, viper.GetBool("log-color")))
			return runServe(cmd.Context(), httpAddr, dbPath, refreshInterval, calendars, logger)
		},
	}
	serve.Flags().String("http-addr", "127.0.0.1:3333", "HTTP listen address")
	serve.Flags().String("config-dir", "./data", "Directory for persistent config, SQLite state, and optional .env")
	serve.Flags().String("db-path", "", "SQLite database path override")
	serve.Flags().Duration("refresh-interval", 5*time.Minute, "Feed refresh interval")
	serve.Flags().String("log-level", "info", "Log level: debug, info, warn, or error")
	serve.Flags().Bool("log-color", true, "Colorize slog output")
	serve.Flags().Var(&calendars, "calendar", "Startup calendar in name=url form; repeatable")
	_ = viper.BindPFlag("http-addr", serve.Flags().Lookup("http-addr"))
	_ = viper.BindPFlag("config-dir", serve.Flags().Lookup("config-dir"))
	_ = viper.BindPFlag("db-path", serve.Flags().Lookup("db-path"))
	_ = viper.BindPFlag("refresh-interval", serve.Flags().Lookup("refresh-interval"))
	_ = viper.BindPFlag("log-level", serve.Flags().Lookup("log-level"))
	_ = viper.BindPFlag("log-color", serve.Flags().Lookup("log-color"))
	viper.SetEnvPrefix("ICSMCP")
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	viper.AutomaticEnv()
	root.AddCommand(serve)
	return root
}

func loadEnvFiles(configDir string) {
	_ = gotenv.Load(".env")
	if configDir != "" {
		_ = gotenv.Load(filepath.Join(configDir, ".env"))
	}
}

func resolveDBPath(configDir string, dbPath string) string {
	if strings.TrimSpace(dbPath) != "" {
		return dbPath
	}
	if strings.TrimSpace(configDir) == "" {
		configDir = "."
	}
	return filepath.Join(configDir, "icsmcp.sqlite3")
}

func runServe(ctx context.Context, httpAddr, dbPath string, refreshInterval time.Duration, calendars []string, logger *slog.Logger) error {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create database directory: %w", err)
	}
	store, err := app.OpenStore(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	svc := app.NewService(store, app.ServiceOptions{RefreshInterval: refreshInterval, Logger: logger})
	if err := svc.ImportStartupCalendars(ctx, app.EnvMap(), calendars); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	go svc.RunRefresher(ctx)

	server := &http.Server{
		Addr:              httpAddr,
		Handler:           app.NewHTTPHandler(svc, app.NewMCPServer(svc)),
		ReadHeaderTimeout: 10 * time.Second,
	}
	printStartupInfo(os.Stdout, httpAddr)
	logger.Info("server starting", "http_addr", httpAddr, "db_path", dbPath, "refresh_interval", refreshInterval.String())
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve http: %w", err)
	}
	return nil
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level %q: use debug, info, warn, or error", value)
	}
}

func printStartupInfo(w io.Writer, httpAddr string) {
	baseURL := "http://" + httpAddr
	if _, err := fmt.Fprintf(w, "ICS MCP server listening on %s\n", httpAddr); err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "Admin UI: %s/\n", baseURL)
	_, _ = fmt.Fprintf(w, "MCP endpoint: %s/mcp\n", baseURL)
	_, _ = fmt.Fprintf(w, "Status API: %s/api/status\n", baseURL)
}
