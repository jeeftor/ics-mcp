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
- SQLite database: `./data/icsmcp.sqlite3`
- Refresh interval: `5m`
- Log level: `info`

Useful flags:

```bash
go run main.go serve \
  --http-addr 0.0.0.0:3333 \
  --db-path ./data/icsmcp.sqlite3 \
  --refresh-interval 5m \
  --log-level debug \
  --calendar MITRE=https://example.invalid/calendar.ics
```

## Calendar Config

Keep `.env` private because ICS feed URLs often contain bearer-style access tokens.

Startup calendars can be loaded from environment variables:

```dotenv
ICSMCP_CALENDAR_MITRE=https://example.invalid/mitre.ics
ICSMCP_CALENDAR_EMILY=https://example.invalid/emily.ics
ICSMCP_LOG_LEVEL=info
ICSMCP_LOG_COLOR=true
```

The suffix after `ICSMCP_CALENDAR_` is the stable key. Underscores are shown as spaces by default. Calendars from `.env` and `--calendar name=url` are upserted on startup; calendars added in the UI, API, or MCP tools are not deleted just because they are absent from startup config.

After startup, SQLite is the runtime source of truth for display names, enabled state, refresh state, and cached event instances.

## HTTP API

- `GET /api/status`
- `GET /api/calendars`
- `POST /api/calendars`
- `PATCH /api/calendars/{id}`
- `DELETE /api/calendars/{id}`
- `POST /api/calendars/{id}/refresh`

## MCP Tools

- `upcoming_meetings`
- `upcoming_meetings_by_calendar`
- `calendar_list`
- `calendar_add`
- `calendar_update`
- `calendar_remove`
- `calendar_refresh`

`upcoming_meetings` returns ongoing meetings plus future meetings, sorted by start time. It defaults to 10 meetings and a 30 day lookahead.

`upcoming_meetings_by_calendar` returns the same meeting fields grouped by calendar for clients that prefer a calendar-first view.

## Debug UI

The admin page at `/` is also the local debug interface. It shows the exact same-origin MCP endpoint (`/mcp`), status endpoint, calendar refresh state, a next-meetings preview grouped by calendar, and a tool runner that lists every exposed MCP tool. Select a tool, edit JSON arguments, run it, and inspect syntax-highlighted JSON output.

## Docker

Tagged releases publish multi-architecture images to GitHub Container Registry:

```bash
docker pull ghcr.io/jeeftor/ics-mcp:latest
docker run --rm -p 3333:3333 \
  --env-file .env \
  -v "$PWD/data:/data" \
  ghcr.io/jeeftor/ics-mcp:latest \
  serve --http-addr 0.0.0.0:3333 --db-path /data/icsmcp.sqlite3 --log-level info
```

## Releases

GitHub Actions runs GoReleaser for tags matching `v*`.

```bash
git tag v0.1.0
git push origin v0.1.0
```

The release workflow builds Linux, macOS, and Windows binaries, publishes checksums, and publishes Docker images for `linux/amd64` and `linux/arm64`.

## Development

```bash
make test
```

The default `make` target prints help and does not mutate state.
