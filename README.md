# calendar-api

`calendar-api` is a small self-hosted calendar adapter that sits in front of an existing CalDAV server.

CalDAV remains the source of truth. This project exposes the same safe calendar operations through:

- an HTTP JSON API
- an MCP stdio server

Both transports share the same validation, dry-run behavior, and ETag-based conflict handling.

## Features

HTTP:

- `GET /healthz`
- `GET /calendars`
- `GET /events`
- `GET /events/upcoming`
- `GET /events/{id}`
- `POST /events`
- `PATCH /events/{id}`
- `POST /events/{id}/move`
- `DELETE /events/{id}`
- `GET /availability`

MCP tools:

- `health`
- `list_calendars`
- `list_events`
- `list_upcoming_events`
- `get_event`
- `create_event`
- `update_event`
- `move_event`
- `delete_event`
- `get_availability`

## Requirements

- Go 1.26+
- A running CalDAV instance
- Localhost-only bind address such as `127.0.0.1:8090`

## Configuration

Required environment variables:

- `CALDAV_BASE_URL`
- `CALDAV_USERNAME`
- `CALDAV_PASSWORD`
- `CALENDAR_DEFAULT_NAME`
- `API_BIND_ADDR`

Optional environment variables:

- `DEFAULT_TIMEZONE`

Example values are in [.env.example](/home/vince/Projects/calendar/.env.example).

## Run

Build the binary:

```bash
go build -o calendar-api ./cmd/calendar-api
```

Build the MCP binary:

```bash
go build -o calendar-api-mcp ./cmd/calendar-api-mcp
```

Run the HTTP server:

```bash
CALDAV_BASE_URL=https://caldav.example.com \
CALDAV_USERNAME=you@example.com \
CALDAV_PASSWORD=<FILL IN PASSWORD> \
CALENDAR_DEFAULT_NAME=personal \
API_BIND_ADDR=127.0.0.1:8090 \
DEFAULT_TIMEZONE=Europe/Paris \
./calendar-api
```

Run the MCP server over stdio:

```bash
CALDAV_BASE_URL=https://caldav.example.com \
CALDAV_USERNAME=you@example.com \
CALDAV_PASSWORD=<FILL IN PASSWORD> \
CALENDAR_DEFAULT_NAME=personal \
API_BIND_ADDR=127.0.0.1:8090 \
DEFAULT_TIMEZONE=Europe/Paris \
./calendar-api-mcp
```

The MCP binary writes protocol traffic on stdout, so application logs go to stderr.

Both binaries also accept explicit runtime flags such as:

- `--caldav-base-url`
- `--caldav-username`
- `--caldav-password`
- `--calendar-default-name`
- `--api-bind-addr`
- `--default-timezone`

## MCP Use

The MCP server is intended to be launched by an MCP client over stdio.

It exposes the same calendar behavior as the HTTP API, including:

- dry-run support for create, update, move, and delete
- explicit short error messages
- ETag enforcement for update, move, and delete unless `dryRun` is true

Recommended write flow for agents:

- call `get_event` first to retrieve the current `etag`
- pass that `etag` to `update_event`, `move_event`, or `delete_event`
- use `dryRun: true` if you want a preview before making the write

## API Notes

All responses are JSON.

`GET /events`:

- Defaults to `CALENDAR_DEFAULT_NAME` when `calendar` is omitted.
- If `from` and `to` are provided, the service asks the CalDAV server for events in that range and expands recurring instances for that window.
- If `from` and `to` are omitted, the service lists the calendar objects returned by CalDAV without adding a synthetic time window.

`GET /events/upcoming`:

- Returns the next upcoming events from now.
- The service queries CalDAV over increasing future windows until it has enough events or reaches a hard upper horizon.

Writes:

- `POST /events`, `PATCH /events/{id}`, `POST /events/{id}/move`, and `DELETE /events/{id}` support `dryRun=true`.
- `PATCH`, `move`, and `DELETE` require an ETag for non-dry-run writes.
- You can supply the ETag with `If-Match`.
- `PATCH` and `move` also accept an `etag` field in the JSON body.
- `DELETE` also accepts `?etag=...` as a query parameter.

All-day events:

- Use explicit RFC3339 timestamps.
- For all-day events, `start` and `end` must be midnight boundaries in the chosen timezone.
- `end` remains exclusive.

## Examples

List calendars:

```bash
curl -s http://127.0.0.1:8090/calendars
```

List events in a range:

```bash
curl -s "http://127.0.0.1:8090/events?from=2026-03-24T00:00:00+01:00&to=2026-03-25T00:00:00+01:00"
```

Create an event:

```bash
curl -s http://127.0.0.1:8090/events \
  -H 'Content-Type: application/json' \
  -d '{
    "title": "Gemeente bezoek",
    "description": "Afspraak bij de gemeente.",
    "start": "2026-03-24T12:30:00+01:00",
    "end": "2026-03-24T13:00:00+01:00",
    "allDay": false,
    "timezone": "Europe/Paris",
    "location": ""
  }'
```

Patch an event:

```bash
curl -s -X PATCH "http://127.0.0.1:8090/events/gemeente-bezoek?calendar=wall" \
  -H 'Content-Type: application/json' \
  -H 'If-Match: "abc123"' \
  -d '{
    "title": "Gemeente bezoek verplaatst",
    "start": "2026-03-24T13:00:00+01:00",
    "end": "2026-03-24T13:30:00+01:00"
  }'
```

Move an event:

```bash
curl -s -X POST "http://127.0.0.1:8090/events/gemeente-bezoek/move?calendar=wall" \
  -H 'Content-Type: application/json' \
  -H 'If-Match: "abc123"' \
  -d '{
    "start": "2026-03-24T14:00:00+01:00",
    "end": "2026-03-24T14:30:00+01:00"
  }'
```

Delete an event:

```bash
curl -s -X DELETE "http://127.0.0.1:8090/events/gemeente-bezoek?calendar=wall&etag=%22abc123%22"
```

Availability:

```bash
curl -s "http://127.0.0.1:8090/availability?from=2026-03-24T09:00:00+01:00&to=2026-03-24T18:00:00+01:00&duration_minutes=30"
```

## Testing

Run the test suite with:

```bash
go test ./...
```

Live end-to-end testing against the configured CalDAV server:

```bash
just e2e-http
just e2e-mcp
```

These tests build the real `calendar-api` and `calendar-api-mcp` binaries, run them against the configured CalDAV server, create a dedicated `calendar-api-test` collection if needed, and verify reads and writes through the live upstream.

## Layout

```txt
cmd/calendar-api/
cmd/calendar-api-mcp/
internal/api/
internal/availability/
internal/config/
internal/events/
internal/mcpserver/
internal/caldav/
internal/service/
runit/
```
