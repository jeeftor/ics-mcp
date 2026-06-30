# Changelog

## v1.4.3 - 2026-06-30

### Improved
- Added a dedicated GitHub Actions test workflow so pushes to `master` and pull requests run `go test ./...`.
- Expanded coverage for the debug UI, endpoint rendering, timezone query errors, calendar validation failures, tool previews, and metrics output.

## v1.4.2 - 2026-06-30

### Fixed
- Release binaries now embed Go timezone data so IANA timezone names such as `America/Denver` work in the scratch Docker image.
- Invalid configured timezones now log a warning and explicitly fall back to UTC instead of silently appearing as UTC.

## v1.4.1 - 2026-06-30

### Added
- Meeting query APIs and MCP tools now accept an optional `timezone` parameter for rendering output in a specific IANA timezone without changing server config.
- The debug Info tab now shows runtime config details and copy buttons for endpoint and setup values.

### Fixed
- Microsoft Exchange / Outlook feeds using Windows timezone IDs such as `Eastern Standard Time` and `Mountain Standard Time` now parse into the correct UTC instants instead of being treated as UTC.

## v1.4.0 - 2026-06-29

### Changed
- Removed the duplicate `calendar_*` MCP tool aliases from discovery in favor of canonical verb-first admin tools.
- Renamed the meeting-prep preset from `meeting_agenda` to `next_meetings`.

### Fixed
- The debug UI now keeps the active Info, Calendars, or Tools tab across reloads and refresh actions.
- The sample Docker Compose file no longer overrides `ICSMCP_TIMEZONE` from `/config/.env` with a default `UTC` value.

## v1.3.0 - 2026-06-29

### Improved
- Reworked the debug UI into Info, Calendars, and Tools tabs so setup details, calendar management, and MCP tool previews are easier to scan.

## v1.2.0 - 2026-06-29

### Added
- Preferred verb-first admin tool aliases: `list_calendars`, `add_calendar`, `validate_calendar`, `update_calendar`, `remove_calendar`, and `refresh_calendar`.
- New read/debug tools: `meeting_agenda`, `current_meetings`, `search_meetings`, and `server_status`.
- `refresh_all_calendars` tool for manually refreshing every enabled feed.
- Timezone-aware meeting presentation through `--timezone`, `ICSMCP_TIMEZONE`, or `TZ`.
- Optional external endpoint display through `--external-url` / `ICSMCP_EXTERNAL_URL`.
- `all_day` and `cancelled` meeting output flags, plus `exclude_cancelled` filters in API, MCP tools, and the debug UI.

### Improved
- Adding a calendar through the HTTP API, MCP `calendar_add` tool, or debug UI now attempts an immediate refresh so valid feeds show meetings right away.
- MCP discovery now advertises the expanded tool set and filter options through the official tool schemas.
- README and Docker Compose examples now document timezone, Docker config, and the current tool surface.
- The debug UI now includes a setup panel with internal and external MCP endpoint snippets.

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
