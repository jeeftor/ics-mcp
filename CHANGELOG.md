# Changelog

## v1.6.1 - 2026-07-01

### Added
- Added `GET /api/free-busy` with JSON output by default and Telegram-ready `format` support for direct REST integrations.

### Improved
- Expanded REST coverage for formatted free/busy output without exposing meeting titles.

## v1.6.0 - 2026-07-01

### Added
- Added Telegram-ready meeting and busy-block output formats: `tg-text`, `tg-html`, and `tg-markdownv2`.
- Added `format` query support for REST meeting previews and formatted `text` fields for MCP/tool-preview read outputs.

### Improved
- Expanded output contract coverage for formatted REST responses, MCP structured content, and free/busy formatting.
- Localized RFC3339 build dates in status output and startup logs to the configured display timezone.

## v1.5.2 - 2026-06-30

### Added
- Added MCP resources for server status, calendars, today's meetings, and upcoming meetings.
- Added MCP prompts for daily briefings, meeting prep, availability summaries, and calendar debug reports.
- Added `window` presets for `today`, `tomorrow`, `today_tomorrow`, `next_24h`, `workday`, `rest_of_workday`, `this_week`, `rest_of_week`, and `rest_of_work_week`.
- Added `include_links` and `links_only` meeting query controls.

## v1.5.1 - 2026-06-30

### Added
- Added a `sort` option to meeting queries with `start_time`, `agenda`, `calendar`, and `ongoing_first` modes.

### Changed
- `today_meetings` now defaults to agenda sorting so ongoing and upcoming timed meetings appear before all-day or multi-day blocks.
- Updated MCP tool descriptions, debug UI defaults, and README guidance for agent-efficient sorting, search behavior, and free/busy time windows.

## v1.5.0 - 2026-06-30

### Release Notes
- Promoted the current MCP calendar server work to a minor release after the addition of token-efficient meeting tools, richer calendar debug UI, timezone-aware output, recurrence metadata, meeting join-link extraction, per-calendar general-query inclusion, free/busy output, and faster release automation.

### Included
- Compact-by-default MCP/API meeting output with optional full detail and opt-in descriptions.
- Read tools for next meeting, next meetings, today's meetings, current meetings, search, grouped calendar views, and free/busy availability.
- Admin/debug UI tabs for setup info, calendar configuration, upcoming meeting previews, and MCP tool calls.
- Docker and binary release artifacts built through the optimized GitHub release workflow.

## v1.4.77 - 2026-06-30

### Fixed
- Include long-running meetings in `today_meetings` when they started before the current day but still overlap today.

### Improved
- Added `end_date` to full-detail meeting output so multi-day events can render clear end dates.
- Expanded the Calendars tab meeting preview with status, time, meeting, metadata, and duration columns, including ongoing/cancelled/all-day/recurring/join-link badges.

## v1.4.76 - 2026-06-30

### Improved
- Expanded feature coverage for compact meeting formatting, compact/full JSON decoding, invalid timezone handling, and direct MCP calls for `next_meeting`, `today_meetings`, and `free_busy`.

## v1.4.75 - 2026-06-30

### Changed
- Made MCP/API meeting output compact by default for lower token usage. Use `detail: "full"` or `detail=full` for the verbose field set.

### Added
- Added token-efficient read tools: `next_meeting`, `today_meetings`, and `free_busy`.

### Improved
- Split Docker publishing out of GoReleaser so release binaries and per-architecture Docker images run in parallel.
- Started release tests, GoReleaser, and Docker image jobs concurrently so tests no longer block artifact publishing.
- Moved the arm64 Docker image build to GitHub's native ARM runner to avoid QEMU emulation.
- Added workflow concurrency to cancel stale test and release runs for repeated pushes.
- Made the Docker certificate stage use the build platform to avoid unnecessary arm64 emulation work.
- Changed the debug UI meeting preview to render each calendar in its own table with human-readable durations.

## v1.4.74 - 2026-06-30

### Improved
- Expanded root CLI coverage for success and error exit-code handling without changing command behavior.

## v1.4.73 - 2026-06-30

### Improved
- Expanded parser normalization coverage for skipped nil-time events and generated UID fallbacks.

## v1.4.72 - 2026-06-30

### Improved
- Expanded direct coverage for `UpcomingQuery` JSON decoding, including canonical `in_progress_only`, legacy `only_ongoing`, mixed inputs, and invalid argument types.

## v1.4.71 - 2026-06-30

### Added
- Added `recurring` and `recurrence_id` to cached events and meeting output so agents can tell series instances from one-off events.

### Improved
- Added parser coverage for recurring ICS events with cancelled `RECURRENCE-ID` overrides, including Outlook `X-MICROSOFT-CDO-*` metadata in the fixture.

## v1.4.70 - 2026-06-30

### Changed
- Renamed the ongoing-only meeting filter to `in_progress_only` in HTTP docs, MCP tool schemas, and debug UI defaults.

### Compatibility
- Continued accepting the legacy `only_ongoing` HTTP query parameter and MCP JSON input field.

## v1.4.69 - 2026-06-30

### Improved
- Grouped the admin UI meeting preview by calendar header rows to reduce repeated calendar cells and improve scan efficiency.

## v1.4.68 - 2026-06-30

### Improved
- Expanded calendar selection coverage for corrupt SQLite calendar ID scan failures.

## v1.4.67 - 2026-06-30

### Improved
- Expanded calendar selection coverage for SQLite calendar ID listing failures.

## v1.4.66 - 2026-06-30

### Fixed
- Report unrecognized ICS `TZID` values instead of silently accepting potentially shifted meeting times.

### Improved
- Expanded parser coverage for unknown timezone identifiers in ICS event fields.

## v1.4.65 - 2026-06-30

### Improved
- Expanded root CLI entrypoint coverage for command execution and error output.

## v1.4.64 - 2026-06-30

### Improved
- Expanded `runServe` coverage for HTTP listener startup failures.

## v1.4.63 - 2026-06-30

### Improved
- Expanded Cobra `serve` command coverage for startup configuration, logging flags, and startup calendar validation failures.

## v1.4.62 - 2026-06-30

### Improved
- Expanded integration coverage for the combined HTTP server's `/mcp` Streamable HTTP endpoint.

## v1.4.61 - 2026-06-30

### Improved
- Expanded official MCP SDK integration coverage for the `refresh_calendar` admin tool.

## v1.4.60 - 2026-06-30

### Improved
- Expanded ICS parser regression coverage for title-prefixed cancellations and missing-start parse failures.

## v1.4.59 - 2026-06-30

### Improved
- Shortened release workflow runtime by removing the duplicate raw-binary rebuild job and relying on GoReleaser archives plus Docker images.
- Updated release documentation and generated release-note artifact text to match the current artifact set.

## v1.4.58 - 2026-06-30

### Improved
- Expanded CLI logging coverage for slog handler writer failures and zero timestamp fallback.

## v1.4.57 - 2026-06-30

### Improved
- Expanded coverage for calendar-selection clear failures and event-cache replacement startup errors.

## v1.4.56 - 2026-06-30

### Improved
- Expanded calendar-selection coverage for backend failures and transaction rollback.

## v1.4.55 - 2026-06-30

### Added
- Added `GET`/`PUT /api/calendars/general-query-selection` for saving the calendars used by default generalized meeting queries.
- Added bulk default-query calendar selection controls to the admin UI.

## v1.4.54 - 2026-06-30

### Improved
- Expanded MCP integration coverage for grouped upcoming-meeting tool calls.

## v1.4.53 - 2026-06-30

### Improved
- Expanded startup coverage for SQLite open and migration failures.

## v1.4.52 - 2026-06-30

### Improved
- Expanded startup output coverage for writer failure handling.

## v1.4.51 - 2026-06-30

### Improved
- Expanded startup coverage for database directory creation failures.

## v1.4.50 - 2026-06-30

### Improved
- Expanded HTTP API coverage for unsupported methods on the meeting preview endpoint.

## v1.4.49 - 2026-06-30

### Improved
- Expanded grouped upcoming-meeting coverage for invalid timezone overrides.

## v1.4.48 - 2026-06-30

### Improved
- Expanded refresh coverage for SQLite refresh-state lookup failures.

## v1.4.47 - 2026-06-30

### Improved
- Expanded validation coverage for calendar feed transport failures.

## v1.4.46 - 2026-06-30

### Added
- Added per-calendar `include_in_general_queries` configuration in SQLite, the REST API, MCP admin tools, and the debug UI.
- Default upcoming-meeting queries now omit calendars opted out of general queries while explicit calendar filters still include them.

## v1.4.45 - 2026-06-30

### Improved
- Expanded event query coverage for corrupt cached event row scan failures.

## v1.4.44 - 2026-06-30

### Improved
- Expanded refresh coverage for event-cache replacement failures and rollback behavior.

## v1.4.43 - 2026-06-30

### Improved
- Expanded SQLite refresh-state and calendar-status scan failure coverage.

## v1.4.42 - 2026-06-30

### Improved
- Expanded calendar listing coverage for corrupt SQLite row scan failures.
- Added clearer error context when listing calendars fails while scanning a row.

## v1.4.41 - 2026-06-30

### Improved
- Logged version, commit, and build date in the structured server startup log.

## v1.4.40 - 2026-06-30

### Improved
- Expanded HTTP API coverage for readiness and metrics backend error responses.
- Changed the debug UI upcoming-meetings preview to show friendly dates and 12-hour AM/PM times.

## v1.4.39 - 2026-06-30

### Improved
- Expanded HTTP API coverage for successful manual calendar refreshes.

## v1.4.38 - 2026-06-30

### Improved
- Expanded startup calendar import coverage for SQLite persistence failures.

## v1.4.37 - 2026-06-30

### Improved
- Expanded SQLite calendar update coverage for URL-only edits and persisted state.

## v1.4.36 - 2026-06-30

### Improved
- Expanded SQLite calendar upsert coverage for insert failures, update failures, and preserved enabled state.

## v1.4.35 - 2026-06-30

### Improved
- Expanded refresh-all coverage for status listing failures.
- Added parser coverage for ignoring events that have a start time but no end time.

## v1.4.34 - 2026-06-30

### Improved
- Expanded SQL placeholder helper coverage for single and repeated parameter lists.

## v1.4.33 - 2026-06-30

### Improved
- Expanded meeting URL classifier coverage for malformed raw URL inputs.

## v1.4.32 - 2026-06-30

### Changed
- Updated Docker run directions to keep timezone and external URL in mounted `config/.env` instead of passing app config as container environment variables.

## v1.4.31 - 2026-06-30

### Improved
- Expanded add-and-refresh coverage for calendar validation failures.

## v1.4.30 - 2026-06-30

### Improved
- Expanded tool argument decoding coverage for empty tool-call payloads.

## v1.4.29 - 2026-06-30

### Improved
- Expanded HTTP JSON helper coverage for response encoding failures.

## v1.4.28 - 2026-06-30

### Improved
- Expanded due-refresh coverage for status scan failures and the warning log path.

## v1.4.27 - 2026-06-30

### Fixed
- Made calendar status listing tolerate a missing refresh-state row and return zero-value refresh fields instead of a scan error.

### Improved
- Expanded SQLite status coverage for calendars with missing refresh-state metadata.

## v1.4.26 - 2026-06-30

### Changed
- Defaulted display timezone to UTC when `ICSMCP_TIMEZONE` and `--timezone` are unset, making container-level `TZ` unnecessary and ignored.

## v1.4.25 - 2026-06-30

### Improved
- Expanded SQLite event-cache replacement coverage for clear failures and transaction rollback after partial insert failures.

## v1.4.24 - 2026-06-30

### Improved
- Expanded calendar validation coverage for default lookahead, default preview limit, sorted preview output, and result trimming.

## v1.4.23 - 2026-06-30

### Improved
- Expanded CLI serve coverage for clean startup and context-cancelled shutdown on an ephemeral HTTP listener.

## v1.4.22 - 2026-06-30

### Improved
- Expanded SQLite mutation coverage for calendar update execution failures, refresh-state insert failures, and duplicate cached-event insert failures.

## v1.4.21 - 2026-06-30

### Improved
- Expanded HTTP meeting query coverage for all supported filters, time windows, description options, calendar IDs, and timezone parsing.
- Expanded meeting description coverage for the default 300-character truncation path.

## v1.4.20 - 2026-06-30

### Improved
- Expanded SQLite store coverage for calendar deletion failures across events, refresh state, and calendar tables.
- Expanded cached event query coverage for corrupt start and end timestamps.

## v1.4.19 - 2026-06-30

### Improved
- Expanded meeting description coverage for opt-in, exact-length, and truncated output.
- Expanded refresh coverage for parse failures while preserving the last known good event cache.

## v1.4.18 - 2026-06-30

### Improved
- Expanded parser coverage for untitled ICS events and missing-UID parse failures.
- Expanded debug tool-preview coverage for malformed JSON arguments across all argument-taking preview tools.

## v1.4.17 - 2026-06-30

### Improved
- Expanded service-layer coverage for closed SQLite store error propagation across calendar, meeting, status, metrics, and refresh operations.

## v1.4.16 - 2026-06-30

### Fixed
- Removed stale `TZ` wording from the `--timezone` CLI help now that app timezone configuration uses `ICSMCP_TIMEZONE` or `--timezone`.

### Improved
- Expanded HTTP API edge-case coverage for method errors, grouped meeting query parsing, calendar item routing, and unknown admin paths.

## v1.4.15 - 2026-06-30

### Changed
- Removed the generic `TZ` environment variable fallback from app timezone configuration; use `ICSMCP_TIMEZONE` or `--timezone` for display times.

## v1.4.14 - 2026-06-30

### Improved
- Expanded logging coverage for colored slog level rendering, empty attribute handling, and handler slice cloning.

## v1.4.13 - 2026-06-30

### Improved
- Expanded CLI and logging coverage for repeatable calendar flags, invalid serve log levels, startup DB directory creation, DB path defaults, startup URL output, and structured slog attributes/groups.

## v1.4.12 - 2026-06-30

### Improved
- Expanded debug tool-preview coverage for meeting preset filters and service error coverage for validation and refresh request/read failures.

## v1.4.11 - 2026-06-30

### Improved
- Expanded service coverage for add-and-refresh failures, idempotent calendar removal, and refresh-all success/failure/skipped summaries.

## v1.4.10 - 2026-06-30

### Improved
- Expanded store and HTTP helper coverage for boolean query parsing, SQLite open failures, duplicate event rollback, and refresh-state timestamp parsing.

## v1.4.9 - 2026-06-30

### Improved
- Expanded service persistence coverage for calendar removal, disabled-calendar query behavior, and calendar validation parse failures.
- Clarified Docker timezone configuration: use `ICSMCP_TIMEZONE` for ics-mcp display times; the app container does not need `TZ`.

## v1.4.8 - 2026-06-30

### Improved
- Expanded meeting URL extraction coverage for Webex links, generic join links, escaped newline cleanup, invalid URL skipping, and empty text.
- Added calendar input validation coverage for missing URLs, missing keys/names, and punctuation-only keys.

## v1.4.7 - 2026-06-30

### Improved
- Expanded service coverage for the long-running background refresher loop, context cancellation, and calendar-ID filtered meeting queries.

## v1.4.6 - 2026-06-30

### Improved
- Expanded refresh behavior coverage for conditional `ETag` / `Last-Modified` requests, `304 Not Modified` refreshes, due-calendar scheduling, future refresh skips, and disabled calendar skips.

## v1.4.5 - 2026-06-30

### Improved
- Expanded HTTP API edge-case coverage for bad query parameters, invalid JSON bodies, method errors, unknown debug tools, and missing tool routes.
- Added startup configuration coverage for repeatable CLI calendars, invalid `name=url` assignments, normalized display names, and process environment loading.

## v1.4.4 - 2026-06-30

### Improved
- Expanded debug tool-preview test coverage across read tools, admin tools, validation, refresh, calendar updates, removal, argument decoding, and unknown-tool errors.

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
