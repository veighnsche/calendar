set shell := ["bash", "-lc"]

# List the canonical recipe surface.
[private]
default:
    @just --list --no-aliases --unsorted

# Launch Codex as the repo-local calendar assistant with the calendar MCP server attached.
cal-agent *args:
    bash -lc 'set -euo pipefail; project_root="{{ justfile_directory() }}"; export PATH="$HOME/.bun/bin:$HOME/.local/bin:$HOME/bin:$PATH"; command -v codex >/dev/null 2>&1 || { echo "Codex CLI not found on PATH" >&2; exit 127; }; command -v go >/dev/null 2>&1 || { echo "Go not found on PATH" >&2; exit 127; }; if [ -f "$project_root/.env" ]; then set -a; . "$project_root/.env"; set +a; fi; caldav_base_url="${CALDAV_BASE_URL:-https://caldav.example.com}"; caldav_username="${CALDAV_USERNAME:-you@example.com}"; caldav_password="${CALDAV_PASSWORD:-<FILL IN PASSWORD>}"; calendar_default_name="${CALENDAR_DEFAULT_NAME:-personal}"; api_bind_addr="${API_BIND_ADDR:-127.0.0.1:8090}"; default_timezone="${DEFAULT_TIMEZONE:-${TZ:-Europe/Paris}}"; [ -n "$caldav_base_url" ] || { echo "Missing required config: CALDAV_BASE_URL" >&2; exit 1; }; [ -n "$caldav_username" ] || { echo "Missing required config: CALDAV_USERNAME" >&2; exit 1; }; [ -n "$caldav_password" ] || { echo "Missing required config: CALDAV_PASSWORD" >&2; exit 1; }; mkdir -p "$project_root/.bin"; (cd "$project_root" && go build -o "$project_root/.bin/calendar-api-mcp" ./cmd/calendar-api-mcp); exec codex --dangerously-bypass-approvals-and-sandbox -C "$project_root" -c "mcp_servers.calendar-api.command=\"$project_root/.bin/calendar-api-mcp\"" -c "mcp_servers.calendar-api.args=[\"--caldav-base-url\",\"$caldav_base_url\",\"--caldav-username\",\"$caldav_username\",\"--caldav-password\",\"$caldav_password\",\"--calendar-default-name\",\"$calendar_default_name\",\"--api-bind-addr\",\"$api_bind_addr\",\"--default-timezone\",\"$default_timezone\"]" "$@"' bash {{args}}

# Run the live HTTP end-to-end test flow against the configured CalDAV server.
e2e-http:
    bash -lc 'set -euo pipefail; project_root="{{ justfile_directory() }}"; caldav_env="${CALDAV_ENV_FILE:-$project_root/.env}"; if [ -f "$caldav_env" ]; then set -a; . "$caldav_env"; set +a; fi; export CALENDAR_API_LIVE_E2E=1; export CALDAV_BASE_URL="${CALDAV_BASE_URL:-}"; export CALDAV_USERNAME="${CALDAV_USERNAME:-}"; export CALDAV_PASSWORD="${CALDAV_PASSWORD:-}"; export CALENDAR_E2E_CALENDAR="${CALENDAR_E2E_CALENDAR:-calendar-api-test}"; export DEFAULT_TIMEZONE="${DEFAULT_TIMEZONE:-${TZ:-Europe/Amsterdam}}"; cd "$project_root"; exec go test ./internal/e2e -count=1 -run "^TestLiveHTTPFlow$"'

# Run the live MCP end-to-end test flow against the configured CalDAV server.
e2e-mcp:
    bash -lc 'set -euo pipefail; project_root="{{ justfile_directory() }}"; caldav_env="${CALDAV_ENV_FILE:-$project_root/.env}"; if [ -f "$caldav_env" ]; then set -a; . "$caldav_env"; set +a; fi; export CALENDAR_API_LIVE_E2E=1; export CALDAV_BASE_URL="${CALDAV_BASE_URL:-}"; export CALDAV_USERNAME="${CALDAV_USERNAME:-}"; export CALDAV_PASSWORD="${CALDAV_PASSWORD:-}"; export CALENDAR_E2E_CALENDAR="${CALENDAR_E2E_CALENDAR:-calendar-api-test}"; export DEFAULT_TIMEZONE="${DEFAULT_TIMEZONE:-${TZ:-Europe/Amsterdam}}"; cd "$project_root"; exec go test ./internal/e2e -count=1 -run "^TestLiveMCPFlow$"'

# Run the full live E2E suite against the configured CalDAV server.
e2e-live:
    bash -lc 'set -euo pipefail; project_root="{{ justfile_directory() }}"; caldav_env="${CALDAV_ENV_FILE:-$project_root/.env}"; if [ -f "$caldav_env" ]; then set -a; . "$caldav_env"; set +a; fi; export CALENDAR_API_LIVE_E2E=1; export CALDAV_BASE_URL="${CALDAV_BASE_URL:-}"; export CALDAV_USERNAME="${CALDAV_USERNAME:-}"; export CALDAV_PASSWORD="${CALDAV_PASSWORD:-}"; export CALENDAR_E2E_CALENDAR="${CALENDAR_E2E_CALENDAR:-calendar-api-test}"; export DEFAULT_TIMEZONE="${DEFAULT_TIMEZONE:-${TZ:-Europe/Amsterdam}}"; cd "$project_root"; exec go test ./internal/e2e -count=1'
