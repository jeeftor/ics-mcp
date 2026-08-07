#!/usr/bin/env sh
# Smoke the production embedded console in a real headless browser.
set -eu

browser_smoke_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
browser_smoke_output="$browser_smoke_root/output/playwright"
browser_smoke_temp=$(mktemp -d "${TMPDIR:-/tmp}/icsmcp-browser-smoke.XXXXXX")
browser_smoke_gocache=${GOCACHE:-"$browser_smoke_temp/go-cache"}
export npm_config_cache=${npm_config_cache:-"$browser_smoke_temp/npm-cache"}
browser_smoke_port=${ICSMCP_BROWSER_SMOKE_PORT:-}
browser_smoke_session="icsmcp-browser-smoke-$$"
browser_smoke_server_pid=""
browser_smoke_cli=""

cleanup() {
  if [ -n "$browser_smoke_server_pid" ]; then
    kill "$browser_smoke_server_pid" 2>/dev/null || true
    wait "$browser_smoke_server_pid" 2>/dev/null || true
  fi
  if [ -n "$browser_smoke_cli" ] && command -v "$browser_smoke_cli" >/dev/null 2>&1; then
    "$browser_smoke_cli" -s="$browser_smoke_session" close >/dev/null 2>&1 || true
  fi
  rm -rf "$browser_smoke_temp"
}
trap cleanup EXIT INT TERM

if ! command -v npx >/dev/null 2>&1; then
  echo "browser smoke requires Node.js/npm (npx)" >&2
  exit 1
fi

if ! command -v pnpm >/dev/null 2>&1 || ! command -v go >/dev/null 2>&1; then
  echo "browser smoke requires pnpm and Go" >&2
  exit 1
fi

browser_smoke_home=${CODEX_HOME:-"$HOME/.codex"}
browser_smoke_cli=${ICSMCP_PLAYWRIGHT_CLI:-"$browser_smoke_home/skills/playwright/scripts/playwright_cli.sh"}
if [ ! -x "$browser_smoke_cli" ]; then
  echo "Playwright CLI wrapper not found at $browser_smoke_cli" >&2
  echo "Set ICSMCP_PLAYWRIGHT_CLI to a playwright-cli command or install the Codex Playwright skill." >&2
  exit 1
fi

if [ -z "$browser_smoke_port" ]; then
  browser_smoke_port=$(node -e 'const net=require("net"); const server=net.createServer(); server.listen(0,"127.0.0.1",()=>{console.log(server.address().port);server.close();});')
fi
browser_smoke_url="http://127.0.0.1:$browser_smoke_port"
mkdir -p "$browser_smoke_output"

snapshot="$browser_smoke_output/browser-smoke-snapshot.yml"

take_snapshot() {
  "$browser_smoke_cli" -s="$browser_smoke_session" snapshot >"$snapshot"
}

expect_text() {
  if ! rg -Fq -- "$1" "$snapshot"; then
    echo "browser smoke expected text: $1" >&2
    cat "$snapshot" >&2
    exit 1
  fi
}

button_ref() {
  awk -v label="$1" '
    index($0, "button") && index($0, label) && match($0, /\[ref=[^]]+\]/) {
      ref = substr($0, RSTART + 5, RLENGTH - 6)
      print ref
      exit
    }
  ' "$snapshot"
}

click_button() {
  take_snapshot
  browser_smoke_ref=$(button_ref "$1")
  if [ -z "$browser_smoke_ref" ]; then
    echo "browser smoke could not find button: $1" >&2
    cat "$snapshot" >&2
    exit 1
  fi
  "$browser_smoke_cli" -s="$browser_smoke_session" click "$browser_smoke_ref" >/dev/null
}

expect_planner_alignment() {
  browser_smoke_width=$1
  "$browser_smoke_cli" -s="$browser_smoke_session" resize "$browser_smoke_width" 900 >/dev/null
  browser_smoke_alignment=$("$browser_smoke_cli" -s="$browser_smoke_session" eval '
    (() => {
      const headings = Array.from(document.querySelectorAll(".day-heading"));
      const tracks = Array.from(document.querySelectorAll(".all-day-day-track"));
      const allDay = document.querySelector(".all-day-grid");
      const timed = document.querySelector(".week-grid");
      if (!headings.length || headings.length !== tracks.length || !allDay || !timed) return "planner-alignment-missing-elements";
      const first = headings[0].getBoundingClientRect();
      const allDayRect = allDay.getBoundingClientRect();
      const timedRect = timed.getBoundingClientRect();
      const dayWidth = timedRect.width / headings.length;
      const close = (a, b) => Math.abs(a - b) < 0.5;
      const aligned = close(first.left, allDayRect.left) && close(first.left, timedRect.left) && close(allDayRect.width, timedRect.width) &&
        headings.every((heading, index) => {
          const headingRect = heading.getBoundingClientRect();
          const trackRect = tracks[index].getBoundingClientRect();
          return close(headingRect.left, timedRect.left + dayWidth * index) && close(headingRect.width, dayWidth) && close(headingRect.left, trackRect.left) && close(headingRect.width, trackRect.width);
        });
      return aligned ? "planner-alignment-pass" : JSON.stringify({ first: first.left, allDay: allDayRect.left, timed: timedRect.left, dayWidth, headings: headings.map(heading => ({ left: heading.getBoundingClientRect().left, width: heading.getBoundingClientRect().width })), tracks: tracks.map(track => ({ left: track.getBoundingClientRect().left, width: track.getBoundingClientRect().width })) });
    })()
  ')
  if ! printf '%s\n' "$browser_smoke_alignment" | rg -Fq 'planner-alignment-pass'; then
    echo "planner columns were not aligned at ${browser_smoke_width}px" >&2
    printf '%s\n' "$browser_smoke_alignment" >&2
    exit 1
  fi
}

cd "$browser_smoke_root"
pnpm -C ui build >/dev/null
GOCACHE="$browser_smoke_gocache" go build -o "$browser_smoke_temp/icsmcp" main.go
(
  cd "$browser_smoke_temp"
  env -i PATH="$PATH" HOME="$HOME" "$browser_smoke_temp/icsmcp" serve \
    --http-addr "127.0.0.1:$browser_smoke_port" \
    --config-dir "$browser_smoke_temp/config" \
    --update-check=false \
    --log-color=false
) >"$browser_smoke_temp/server.log" 2>&1 &
browser_smoke_server_pid=$!

browser_smoke_attempt=0
until curl -fsS "$browser_smoke_url/healthz" >/dev/null 2>&1; do
  browser_smoke_attempt=$((browser_smoke_attempt + 1))
  if [ "$browser_smoke_attempt" -ge 50 ]; then
    echo "browser smoke server did not become ready" >&2
    cat "$browser_smoke_temp/server.log" >&2
    exit 1
  fi
  sleep 0.1
done

"$browser_smoke_cli" -s="$browser_smoke_session" open "$browser_smoke_url/" >/dev/null
take_snapshot
expect_text "7-DAY CALENDAR"
expect_text "Calendar week"
for browser_smoke_width in 1440 1100 900 769; do
  expect_planner_alignment "$browser_smoke_width"
done
click_button "Calendar week"
take_snapshot
expect_text "CALENDAR WEEK"
click_button "Agenda"
take_snapshot
expect_text "UPCOMING MEETINGS"
expect_text "Group events"
click_button "7 days"
take_snapshot
expect_text "7-DAY CALENDAR"

"$browser_smoke_cli" -s="$browser_smoke_session" goto "$browser_smoke_url/config/environment" >/dev/null
take_snapshot
expect_text "Environment overrides"

"$browser_smoke_cli" -s="$browser_smoke_session" goto "$browser_smoke_url/api/mcp-tools" >/dev/null
take_snapshot
expect_text "MCP tools"

"$browser_smoke_cli" -s="$browser_smoke_session" goto "$browser_smoke_url/ai" >/dev/null
take_snapshot
expect_text "Grounded calendar briefings"
expect_text "AI CONNECTION"
expect_text "Scheduled and on-demand questions"

"$browser_smoke_cli" -s="$browser_smoke_session" resize 390 844 >/dev/null
"$browser_smoke_cli" -s="$browser_smoke_session" goto "$browser_smoke_url/" >/dev/null
take_snapshot
expect_text "Agenda"
expect_text "Mobile calendar view"
click_button "Day"
take_snapshot
expect_text "No events on this day"
click_button "Calendar filters"
take_snapshot
expect_text "CALENDAR FILTERS"
expect_text "Calendars"
click_button "Done"

echo "Browser smoke passed: planner alignment across desktop widths, SPA routes, and mobile agenda/day/context controls."
