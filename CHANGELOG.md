# Changelog

## v1.1.1 - 2026-06-29

### Fixed
- Empty calendar and meeting collections now encode as `[]` instead of `null`, preventing the debug UI from failing on `groups.length` when no calendars or cached meetings exist.
- The debug UI now defensively treats unexpected `null` list responses as empty arrays.

## v1.1.0 - 2026-06-29

### Added
- `icsmcp version` command and build metadata in `/api/status`, MCP server metadata, and the debug UI.
- Health, readiness, and Prometheus-compatible metrics endpoints at `/healthz`, `/readyz`, and `/metrics`.
- Calendar feed validation through `POST /api/calendars/validate`, the `calendar_validate` MCP tool, and the debug tool runner.
- Meeting filters for `query`, `only_ongoing`, `exclude_all_day`, `after`, and `before`.
- Docker Compose example and clearer Docker deployment notes.

### Improved
- Release builds now inject version, commit, and build date into GoReleaser archives/images and raw binaries.
- The debug UI links the health and metrics endpoints and displays the running version.
- GitHub Actions release workflow pins current action and GoReleaser versions to avoid deprecation and floating-version warnings.

## v1.0.0 - 2026-06-29

### Added
- Streamable HTTP MCP server at `/mcp` with calendar read tools and calendar admin tools.
- Embedded admin/debug UI at `/` for calendar management, refresh state, upcoming-meeting previews, MCP tool discovery, and JSON tool-call previews.
- SQLite-backed calendar config, refresh metadata, and normalized upcoming-event cache.
- Startup calendar import from `ICSMCP_CALENDAR_<KEY>` environment variables, `.env` files, and repeatable `--calendar name=url` flags.
- Persistent `--config-dir` support for local and Docker deployments, including `/config` as the container mount point.
- `upcoming_meetings` and `upcoming_meetings_by_calendar` tools with compact day labels, optional descriptions, and join-link extraction for common meeting providers.
- Colored structured logs with configurable `--log-level`, `ICSMCP_LOG_LEVEL`, and `--log-color`.
- Multi-architecture release pipeline for GitHub Release archives, raw binaries, and GHCR Docker images.

### Improved
- Meeting output is token-conscious by default: descriptions are omitted unless requested and day labels use compact names such as `Mon`.
- The debug UI shows the same-origin MCP endpoint, calendar IDs, grouped upcoming meetings, and collapsible formatted JSON.
- Docker deployment keeps feed config and SQLite state under a single mounted config directory.

### Notes
- v1 is intentionally unauthenticated. Run it only on a trusted local or homelab network, or put it behind your own reverse proxy and access control.
