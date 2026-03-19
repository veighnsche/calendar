# calendar-api

`calendar-api` is a small self-hosted JSON API that sits in front of an existing Radicale CalDAV server.

Radicale remains the source of truth. This service only adapts it into a stable, agent-friendly HTTP API with explicit JSON responses, dry-run support, and ETag-based conflict handling.

## Features

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

## Requirements

- Go 1.26+
- A running Radicale instance
- Localhost-only bind address such as `127.0.0.1:8090`

## Configuration

Required environment variables:

- `RADICALE_BASE_URL`
- `RADICALE_USERNAME`
- `RADICALE_PASSWORD`
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

Run it:

```bash
set -a
. ./.env.example
set +a
./calendar-api
```

## API Notes

All responses are JSON.

`GET /events`:

- Defaults to `CALENDAR_DEFAULT_NAME` when `calendar` is omitted.
- If `from` and `to` are provided, the service asks Radicale for events in that range and expands recurring instances for that window.
- If `from` and `to` are omitted, the service lists the calendar objects returned by Radicale without adding a synthetic time window.

`GET /events/upcoming`:

- Returns the next upcoming events from now.
- The service queries Radicale over increasing future windows until it has enough events or reaches a hard upper horizon.

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

## Layout

```txt
cmd/calendar-api/
internal/api/
internal/availability/
internal/config/
internal/events/
internal/radicale/
runit/
```
