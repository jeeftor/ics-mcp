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
- Display timezone: `ICSMCP_TIMEZONE`, then `TZ`, then the host local timezone

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
- `GET /api/tools`
- `POST /api/tools/{name}/call`
- `GET /api/calendars`
- `POST /api/calendars`
- `POST /api/calendars/validate`
- `PATCH /api/calendars/{id}`
- `DELETE /api/calendars/{id}`
- `POST /api/calendars/{id}/refresh`

Meeting preview endpoints accept `limit`, `lookahead_days`, repeated `calendar_id`, `query`, `timezone`, `only_ongoing`, `exclude_all_day`, `exclude_cancelled`, `include_description`, `description_max_chars`, `after`, and `before`. `timezone` is optional and accepts IANA names such as `America/Denver` or `UTC`; when omitted, output uses the configured display timezone. `after` and `before` use RFC3339 timestamps.

`/healthz` is the liveness endpoint and `/readyz` is the readiness endpoint. The `z` suffix is a common convention from Kubernetes-style health checks.

## MCP Tools

- `upcoming_meetings`
- `upcoming_meetings_by_calendar`
- `next_meetings`
- `current_meetings`
- `search_meetings`
- `server_status`
- `list_calendars`
- `add_calendar`
- `validate_calendar`
- `update_calendar`
- `remove_calendar`
- `refresh_calendar`
- `refresh_all_calendars`

`upcoming_meetings` returns ongoing meetings plus future meetings, sorted by start time. It defaults to 10 meetings and a 30 day lookahead. Day labels are compact (`Mon`, `Tue`, etc.), display times are rendered in the configured timezone, and descriptions are omitted by default. Use `timezone` to render a specific query in another IANA timezone. Use `include_description: true` and optional `description_max_chars` when details are needed. Optional filters include `query`, `only_ongoing`, `exclude_all_day`, `exclude_cancelled`, `calendar_ids`, `after`, and `before`.

`upcoming_meetings_by_calendar` returns the same meeting fields grouped by calendar for clients that prefer a calendar-first view. Its `limit` applies per calendar, so the default is 10 meetings per calendar.

`next_meetings` is the opinionated, token-conscious preset for normal meeting prep. It returns the same shape as `upcoming_meetings`, but always excludes all-day blocks and cancelled events.

`current_meetings` returns only events that have already started and have not ended yet. Ongoing events are marked with `ongoing: true`.

`search_meetings` uses the same cached event data and filters as `upcoming_meetings`; pass `query` to match title, description, or calendar name.

`server_status` returns build metadata, timezone, optional external URL, and calendar refresh state.

The admin tools use one canonical verb-first naming style: `list_calendars`, `add_calendar`, `validate_calendar`, `update_calendar`, `remove_calendar`, `refresh_calendar`, and `refresh_all_calendars`.

`validate_calendar` fetches and parses an ICS feed without saving it. It returns fetch status, event count, and a small upcoming-meeting preview so you can test a URL before adding it.

Meeting outputs include `timezone`, `all_day`, `cancelled`, `meeting_url`, and `meeting_url_type`. `meeting_url` is set when an online join link can be extracted from ICS `URL`, `LOCATION`, or `DESCRIPTION` fields. Known providers such as Teams, Zoom, Google Meet, and Webex are preferred over generic links.

MCP tool discovery exposes each tool name, description, and JSON input schema. For example, `upcoming_meetings`, `next_meetings`, and `search_meetings` all advertise the `limit`, `calendar_ids`, `lookahead_days`, `timezone`, description, all-day, cancelled, and time-window options through the official `tools/list` response.

## Debug UI

The admin page at `/` is also the local debug interface. It shows the exact same-origin MCP endpoint (`/mcp`), optional external endpoint from `ICSMCP_EXTERNAL_URL`, status endpoint, health endpoint, readiness endpoint, metrics endpoint, runtime config, build version, calendar refresh state, copy buttons for endpoint/setup values, setup snippets for MCP clients, a next-meetings preview grouped by calendar, and a tool runner that lists every exposed MCP tool. Select a tool, edit JSON arguments, run it, and inspect syntax-highlighted JSON output.

## Docker

Tagged releases publish multi-architecture images to GitHub Container Registry:

```bash
docker pull ghcr.io/jeeftor/ics-mcp:latest
docker run --rm -p 3333:3333 \
  -v "$PWD/config:/config" \
  -e ICSMCP_TIMEZONE=America/Denver \
  -e ICSMCP_EXTERNAL_URL=http://192.168.1.112:3333 \
  ghcr.io/jeeftor/ics-mcp:latest \
  serve --http-addr 0.0.0.0:3333 --config-dir /config --log-level info
```

Create `config/.env` before running the container, or pass `ICSMCP_CALENDAR_<KEY>` values through your container runtime. Put `ICSMCP_TIMEZONE` and `ICSMCP_EXTERNAL_URL` there too when the container is reached through a LAN IP, reverse proxy, or non-default port. The `/config` mount preserves the SQLite database and UI/API changes across restarts.

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

The release workflow runs tests, builds Linux, macOS, and Windows archives/checksums with GoReleaser, publishes plain raw binaries named `icsmcp_<os>_<arch>`, and publishes Docker images for `linux/amd64` and `linux/arm64`.

Release artifacts:

- GitHub Release archives: `ics-mcp_<version>_<os>_<arch>.tar.gz` or `.zip`
- Raw binaries: `icsmcp_linux_amd64`, `icsmcp_darwin_arm64`, `icsmcp_windows_amd64.exe`, etc.
- Docker images: `ghcr.io/jeeftor/ics-mcp:<semver-without-v>` and `ghcr.io/jeeftor/ics-mcp:latest`, for example `ghcr.io/jeeftor/ics-mcp:1.2.0`
- Checksums: `checksums.txt`

Update `CHANGELOG.md` before tagging a release.

Release builds inject version, commit, and build date into `icsmcp version`, `/api/status`, the MCP implementation metadata, and the debug UI.

## Development

```bash
make test
```

The default `make` target prints help and does not mutate state.
