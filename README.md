# ICS MCP

ICS MCP is a small Go MCP server for homelab calendar feeds. It watches ICS URLs, stores normalized upcoming event instances in SQLite, exposes a Streamable HTTP MCP endpoint, and includes a simple embedded admin UI.

The v1 server is intentionally unauthenticated. Run it only on a trusted local or homelab network, or put it behind your own reverse proxy and access control.

## Run Locally

```bash
go run main.go serve
```

Defaults:

- HTTP listen address: `127.0.0.1:3333`
- MCP endpoint: `http://127.0.0.1:3333/mcp`
- Admin UI: `http://127.0.0.1:3333/`
- Config directory: `./data`
- SQLite database: `./data/icsmcp.sqlite3`
- Refresh interval: `5m`
- Log level: `info`
- Display timezone: `ICSMCP_TIMEZONE`, then `UTC`

Print build metadata:

```bash
go run main.go version
```

Useful flags:

```bash
go run main.go serve \
  --http-addr 0.0.0.0:3333 \
  --config-dir ./data \
  --refresh-interval 5m \
  --timezone America/Denver \
  --external-url https://ics-mcp.vookie.net \
  --log-level debug \
  --calendar MITRE=https://example.invalid/calendar.ics
```

## Calendar Config

Keep `.env` private because ICS feed URLs often contain bearer-style access tokens.

The server loads an optional `.env` from the config directory first, then an optional `.env` from the current working directory. Existing environment variables are not overwritten, so runtime env values still win over files.

For Docker, put persistent config in `/config`:

```text
/config/.env
/config/icsmcp.sqlite3
```

Startup calendars can be loaded from environment variables:

```dotenv
ICSMCP_CALENDAR_MITRE=https://example.invalid/mitre.ics
ICSMCP_CALENDAR_EMILY=https://example.invalid/emily.ics
ICSMCP_TIMEZONE=America/Denver
ICSMCP_EXTERNAL_URL=http://192.168.1.112:3333
ICSMCP_LOG_LEVEL=info
ICSMCP_LOG_COLOR=true
```

The suffix after `ICSMCP_CALENDAR_` is the stable key. Underscores are shown as spaces by default. Calendars from `.env` and `--calendar name=url` are upserted on startup; calendars added in the UI, API, or MCP tools are not deleted just because they are absent from startup config.

Each calendar also has an `include_in_general_queries` setting, exposed in the Calendars tab, `PATCH /api/calendars/{id}`, and `update_calendar`. It defaults to `true`. Set it to `false` when a calendar should keep refreshing and remain explicitly queryable, but should not appear in default/general upcoming meeting results. Explicit `calendar_id` or `calendar_ids` filters still return that calendar.

After startup, SQLite is the runtime source of truth for display names, enabled state, refresh state, and cached event instances.

Outlook / Exchange feeds commonly use Windows timezone IDs such as `Eastern Standard Time`, `Mountain Standard Time`, and `Pacific Standard Time`; those are mapped to IANA zones during parsing before events are cached.

`ICSMCP_TIMEZONE` accepts IANA names such as `America/Denver`, `UTC`, and common Outlook / Windows timezone IDs such as `Mountain Standard Time`. If a configured timezone is not recognized, startup logs a warning and falls back to `UTC`.

Local config-directory smoke test:

```bash
mkdir -p config
$EDITOR config/.env
go run main.go serve --config-dir ./config --log-level debug
```

The startup output prints the Admin UI, MCP endpoint, status URL, display timezone, and external URL when configured. The SQLite database should appear at `./config/icsmcp.sqlite3`.

## HTTP API

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `GET /api/status`
- `GET /api/meetings`
- `GET /api/meetings/by-calendar`
- `GET /api/rest/{tool_name}`
- `POST /api/rest/{tool_name}`
- `GET /api/events`
- `GET /api/events/by-calendar`
- `GET /api/events/today`
- `GET /api/events/tomorrow`
- `GET /api/events/today-tomorrow`
- `GET /api/events/current`
- `GET /api/events/next`
- `GET /api/events/search`
- `GET /api/free-busy`
- `GET /api/calendars/{calendar}/events`
- `GET /api/calendars/{calendar}/today`
- `GET /openapi.json`
- `GET /docs`
- `GET /api/tools`
- `POST /api/tools/{name}/call`
- `GET /api/calendars`
- `POST /api/calendars`
- `POST /api/calendars/validate`
- `PATCH /api/calendars/{id}`
- `DELETE /api/calendars/{id}`
- `POST /api/calendars/{id}/refresh`

Meeting preview endpoints accept `limit`, `lookahead_days`, repeated `calendar_id`, `calendar`, `query`, `window`, `day`, `range`, `timezone`, `detail`, `sort`, `in_progress_only`, `exclude_all_day`, `exclude_cancelled`, `include_description`, `description_max_chars`, `include_links`, `links_only`, `include_disabled`, `after`, and `before`. When no `calendar_id` is supplied, calendars with `include_in_general_queries=false` are omitted. Disabled calendars are also omitted unless `include_disabled=true` is supplied with an explicit calendar filter. `timezone` is optional and accepts IANA names such as `America/Denver` or `UTC`; when omitted, output uses the configured display timezone. `detail` defaults to compact token-efficient output; use `detail=full` for the verbose field set. `sort` accepts `start_time`, `agenda`, `calendar`, and `ongoing_first`. `window`, `day`, and `range` accept presets such as `today`, `tomorrow`, `today_tomorrow`, `next_24h`, `workday`, `rest_of_workday`, `this_week`, `rest_of_week`, and `rest_of_work_week`. `after` and `before` use RFC3339 timestamps. The older `only_ongoing` query parameter is still accepted for compatibility.

REST read endpoints default to JSON and can also render simple-client formats with either a path extension, `format` query parameter, or `Accept` header: `json`, `html`, `md`, `txt`, `ascii`, and `csv`. `txt` is a plain line-oriented view. `html`, `md`, `ascii`, and `csv` render table-shaped output and accept a comma-separated `fields` selector. The default table fields are `when,calendar,title,duration`; useful alternatives include `duration_minutes`, `ongoing`, `all_day`, `cancelled`, `recurring`, `meeting_url`, `start`, `end`, `timezone`, and `calendar_id`. Table formats hide timezone text by default; pass `show_timezone=true` to include it. Use `time_style` to tune the `when` column: `date_range` (default), `range`, `start`, `date_start`, `time_range`, or `time_start`. Examples:

```bash
curl 'http://localhost:3333/api/events/today?format=txt&limit=5'
curl 'http://localhost:3333/api/events/next.ascii?limit=3&time_style=start'
curl 'http://localhost:3333/api/events/by-calendar?format=md&fields=when,calendar,title,duration&time_style=range&exclude_cancelled=true'
curl 'http://localhost:3333/api/events.csv?fields=when,title&time_style=start&limit=10'
curl 'http://localhost:3333/api/free-busy?range=today_tomorrow&format=json'
curl 'http://localhost:3333/api/events?calendar_id=e81d3050123a252f593bdd01e0e0bd373a596f67&include_disabled=true'
```

`/api/rest/{tool_name}` exposes the same behavior as MCP tools. Use `GET` for read tools such as `upcoming_meetings`, `today_meetings`, `search_meetings`, and `free_busy`; use `POST` with a JSON body for admin tools such as `update_calendar`, `refresh_calendar`, `refresh_all_calendars`, and `validate_calendar`. `/openapi.json` describes the REST surface, and `/docs` links to the REST tab and OpenAPI document.

`/healthz` is the liveness endpoint and `/readyz` is the readiness endpoint. The `z` suffix is a common convention from Kubernetes-style health checks.

## MCP Tools

- `upcoming_meetings`
- `upcoming_meetings_by_calendar`
- `next_meeting`
- `next_meetings`
- `today_meetings`
- `current_meetings`
- `search_meetings`
- `free_busy`
- `server_status`
- `list_calendars`
- `add_calendar`
- `validate_calendar`
- `update_calendar`
- `remove_calendar`
- `refresh_calendar`
- `refresh_all_calendars`

`upcoming_meetings` returns ongoing meetings plus future meetings, sorted by start time unless `sort` is supplied. It defaults to 10 meetings and a 30 day lookahead. Output is compact by default, using `when`, `title`, `calendar`, `duration`, and `duration_minutes`, plus the always-present status flags `ongoing`, `all_day`, `cancelled`, and `recurring`. Meeting URL fields are emitted only when relevant. Pass `detail: "full"` to include separate `day`, `date`, `end_date`, `start`, `end`, `timezone`, calendar IDs, recurrence IDs, and other verbose fields. Descriptions are omitted by default; use `include_description: true` and optional `description_max_chars` when details are needed. Links are included by default when found; pass `include_links: false` to hide them or `links_only: true` to return only meetings with links. Calendars opted out of general queries are omitted unless `calendar_ids` is supplied. Disabled calendars require both explicit `calendar_ids` and `include_disabled: true`. Use `timezone` to render a specific query in another IANA timezone. Optional filters include `query`, `window`, `sort`, `in_progress_only`, `exclude_all_day`, `exclude_cancelled`, `calendar_ids`, `include_disabled`, `after`, and `before`. MCP JSON input still accepts the older `only_ongoing` field for compatibility.

`sort` supports these modes: `start_time` for raw chronological order, `agenda` for ongoing timed meetings then upcoming timed meetings then all-day/multi-day blocks, `calendar` for all-day/multi-day blocks first then timed events, and `ongoing_first` for current events first then chronological order.

`window` presets let agents avoid date arithmetic: `today`, `tomorrow`, `today_tomorrow`, `next_24h`, `workday`, `rest_of_workday`, `this_week`, `rest_of_week`, and `rest_of_work_week`. Day and week windows are resolved in the configured or requested display timezone and include events that overlap the window, including multi-day blocks.

`upcoming_meetings_by_calendar` returns the same meeting fields grouped by calendar for clients that prefer a calendar-first view. Its `limit` applies per calendar, so the default is 10 meetings per calendar. The `sort` option applies within each calendar group.

`next_meeting` returns only the next non-all-day, non-cancelled meeting. Use this when a consumer asks what is next and does not need a larger agenda.

`next_meetings` is the opinionated, token-conscious preset for normal meeting prep. It returns the same shape as `upcoming_meetings`, but always excludes all-day blocks and cancelled events.

`today_meetings` returns meetings for the current display day using the configured timezone or the optional query timezone. It defaults to `sort: "agenda"` so ongoing timed meetings and upcoming timed meetings appear before all-day or multi-day blocks. Use `sort: "calendar"` to show all-day and multi-day blocks first.

`current_meetings` returns only events that have already started and have not ended yet. Ongoing events are marked with `ongoing: true`.

`search_meetings` uses the same cached event data and filters as `upcoming_meetings`; pass `query` to match title, calendar name, or cached description. Descriptions are still omitted from output unless `include_description` is true.

`free_busy` returns busy blocks without meeting titles or descriptions. It is the most privacy-preserving and token-efficient view when a consumer only needs availability. Use `window`, or explicit `after` and `before`, to keep availability checks scoped to a specific window.

`server_status` returns build metadata, timezone, optional external URL, and calendar refresh state.

The admin tools use one canonical verb-first naming style: `list_calendars`, `add_calendar`, `validate_calendar`, `update_calendar`, `remove_calendar`, `refresh_calendar`, and `refresh_all_calendars`.

`validate_calendar` fetches and parses an ICS feed without saving it. It returns fetch status, event count, and a small upcoming-meeting preview so you can test a URL before adding it.

Compact meeting outputs include `when`, `title`, `calendar`, `duration`, `duration_minutes`, `ongoing`, `all_day`, `cancelled`, and `recurring`. The four status flags are always present, including when false, so MCP structured output matches the advertised schema. Optional fields such as `meeting_url` and `meeting_url_type` are emitted only when useful. `recurring` is true for expanded RRULE instances and `RECURRENCE-ID` overrides; pass `detail: "full"` when a consumer needs raw `recurrence_id` values or separate time fields. Cancelled recurring overrides are preserved with `cancelled: true`, then hidden by default when `exclude_cancelled` is enabled. `meeting_url` is set when an online join link can be extracted from ICS `URL`, `LOCATION`, or `DESCRIPTION` fields. Known providers such as Teams, Zoom, Google Meet, and Webex are preferred over generic links.

MCP resources expose stable read-only context at `icsmcp://status`, `icsmcp://calendars`, `icsmcp://meetings/today`, and `icsmcp://meetings/upcoming`. MCP prompts expose `daily_briefing`, `meeting_prep`, `availability_summary`, and `calendar_debug_report`.

MCP tool discovery exposes each tool name, description, and JSON input schema. For example, `upcoming_meetings`, `next_meetings`, and `search_meetings` all advertise the `limit`, `calendar_ids`, `lookahead_days`, `window`, `timezone`, `sort`, description, all-day, cancelled, links, and time-window options through the official `tools/list` response.

## Debug UI

The admin page at `/` is also the local debug interface. It shows the exact same-origin MCP endpoint (`/mcp`), optional external endpoint from `ICSMCP_EXTERNAL_URL`, REST endpoints, OpenAPI link, status endpoint, health endpoint, readiness endpoint, metrics endpoint, runtime config, build version, calendar refresh state, per-calendar general-query inclusion, copy buttons for endpoint/setup values, setup snippets for MCP clients, a dedicated Meetings tab with the next-meetings preview grouped by calendar, a REST tab with generated internal/external URLs, format/layout/field controls, JSON/text previews, and e-paper-style example URLs, and a tool runner that lists every exposed MCP tool. Select a tool, edit JSON arguments, run it, and inspect syntax-highlighted JSON output.

## Docker

Tagged releases publish multi-architecture images to GitHub Container Registry:

```bash
mkdir -p config
$EDITOR config/.env
docker pull ghcr.io/jeeftor/ics-mcp:latest
docker run --rm -p 3333:3333 \
  -v "$PWD/config:/config" \
  ghcr.io/jeeftor/ics-mcp:latest \
  serve --http-addr 0.0.0.0:3333 --config-dir /config --log-level info
```

Create `config/.env` before running the container, or pass `ICSMCP_CALENDAR_<KEY>` values through your container runtime. Put `ICSMCP_TIMEZONE` and `ICSMCP_EXTERNAL_URL` in `config/.env` too when the container is reached through a LAN IP, reverse proxy, or non-default port. The `/config` mount preserves the SQLite database and UI/API changes across restarts.

Use `ICSMCP_TIMEZONE` for ics-mcp display times. The app defaults to `UTC` when no app timezone is configured and intentionally ignores the generic container `TZ` variable, so the container image does not need a `TZ` environment variable.

The repository also includes `compose.yaml`. It does not set `ICSMCP_TIMEZONE` itself, so values from `config/.env` are honored inside the container:

```bash
mkdir -p config
$EDITOR config/.env
docker compose up -d
```

The published image is built from the Go binary into a minimal `scratch` runtime image. It includes the normal public CA certificate bundle so outbound HTTPS calendar feeds work; it does not include private corporate certificate material.

Build the image locally when you want to test the Dockerfile before a release:

```bash
go build -trimpath -o icsmcp ./main.go
docker build -t ics-mcp:local .
docker run --rm -p 3333:3333 \
  -v "$PWD/config:/config" \
  ics-mcp:local
```

## Releases

GitHub Actions runs GoReleaser for tags matching `v*`.

```bash
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

The release workflow optimizes for wall-clock speed: tests, GoReleaser archives/checksums, and per-architecture Docker images all start in parallel. The `linux/arm64` Docker image uses GitHub's native ARM runner to avoid QEMU emulation, then a final manifest step publishes the multi-architecture image tags.

Release artifacts:

- GitHub Release archives: `ics-mcp_<version>_<os>_<arch>.tar.gz` or `.zip`
- Docker images: `ghcr.io/jeeftor/ics-mcp:<semver-without-v>` and `ghcr.io/jeeftor/ics-mcp:latest`, for example `ghcr.io/jeeftor/ics-mcp:1.2.0`
- Checksums: `checksums.txt`

Update `CHANGELOG.md` before tagging a release.

Release builds inject version, commit, and build date into `icsmcp version`, `/api/status`, the MCP implementation metadata, and the debug UI.

## Development

```bash
make test
```

The default `make` target prints help and does not mutate state.
