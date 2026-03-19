# Calendar API Requirements

## Goal

Build a small self-hosted calendar API service that sits between AI agents and an existing CalDAV server.

This service must make calendar management simple, safe, and predictable for agents. The upstream CalDAV server remains the source of truth. The new service provides a clean JSON API and hides CalDAV complexity.

Project location:

- `/home/vince/Projects/calendar`

Existing backend:

- A CalDAV server is already running separately
- Do not replace the upstream server
- Do not store calendar truth anywhere else in v1

## Core Principles

- The upstream CalDAV server is the system of record
- This API is an adapter and agent-safe control layer
- Keep v1 small and reliable
- Prefer boring, explicit behavior over cleverness
- Never fake calendar state
- Every write must be traceable and conflict-aware

## Primary Use Cases

- list upcoming agenda items
- create a new calendar event
- update an event
- move an event
- delete an event
- query free time in a range
- let an AI agent manage the agenda safely through simple tools

## Non-Goals For V1

- no full CalDAV server replacement
- no full groupware suite
- no invitations / attendee workflows
- no recurring-event editing UI beyond basic pass-through support if feasible
- no database required unless there is a very strong reason
- no background job system unless absolutely needed

## Recommended Stack

- language: Go
- HTTP: stdlib `net/http`
- JSON: stdlib `encoding/json`
- logging: `log/slog`
- config: env vars and/or small config file
- CalDAV interaction: either a small direct implementation or a lightweight library if one is clearly stable and reduces risk

Why Go:

- single binary
- reliable long-running service
- simple deployment
- good fit for API middleware

## External Dependency

The service must connect to the existing CalDAV server.

Required config:

- `CALDAV_BASE_URL`
- `CALDAV_USERNAME`
- `CALDAV_PASSWORD`
- `CALENDAR_DEFAULT_NAME`
- `API_BIND_ADDR`

Example:

- `CALDAV_BASE_URL=https://caldav.example.com`
- `CALDAV_USERNAME=you@example.com`
- `CALDAV_PASSWORD=...`
- `CALENDAR_DEFAULT_NAME=personal`
- `API_BIND_ADDR=127.0.0.1:8090`

## API Design

All responses must be JSON.

### Health

- `GET /healthz`

Response:

- service health
- CalDAV reachability summary

### Calendars

- `GET /calendars`

Returns known calendar collections for the configured user.

### Events List

- `GET /events`

Supported query params:

- `calendar`
- `from`
- `to`
- `limit`
- `q`

Behavior:

- returns normalized events in ascending start time
- defaults to the configured default calendar when `calendar` is omitted

### Upcoming

- `GET /events/upcoming`

Supported query params:

- `calendar`
- `limit`

Behavior:

- returns the next N upcoming events from now

### Single Event

- `GET /events/:id`

Returns one normalized event.

### Create

- `POST /events`

Request body:

- `calendar`
- `title`
- `description`
- `start`
- `end`
- `allDay`
- `timezone`
- `location`

Requirements:

- validate required fields
- reject invalid ranges
- support dry-run mode via query param or body flag

### Update

- `PATCH /events/:id`

Can change:

- title
- description
- start
- end
- allDay
- timezone
- location

Requirements:

- optimistic concurrency if possible using etag / revision
- reject invalid updates

### Move

- `POST /events/:id/move`

Request body:

- `start`
- `end`
- optional `calendar`

Purpose:

- dedicated simple endpoint for agent-driven rescheduling

### Delete

- `DELETE /events/:id`

Requirements:

- support dry-run
- require explicit target id

### Availability

- `GET /availability`

Query params:

- `calendar`
- `from`
- `to`
- `duration_minutes`

Response:

- busy intervals
- optionally candidate free slots

## Event Shape

Normalized event shape:

```json
{
  "id": "gemeente-bezoek",
  "calendar": "wall",
  "title": "Gemeente bezoek",
  "description": "Afspraak bij de gemeente.",
  "start": "2026-03-24T12:30:00+01:00",
  "end": "2026-03-24T13:00:00+01:00",
  "allDay": false,
  "timezone": "Europe/Paris",
  "location": "",
  "etag": "\"abc123\"",
  "source": "caldav"
}
```

## Agent-Friendly Behavior

This API is specifically meant to be easy for agents to use.

That means:

- stable endpoint names
- stable response shapes
- explicit error messages
- no HTML
- no CalDAV-specific XML exposed to callers
- no silent fallback behavior

## Error Handling

Errors must be explicit and short.

Examples:

- `calendar not found`
- `event not found`
- `invalid time range`
- `caldav unavailable`
- `write conflict`

HTTP behavior:

- `400` for bad input
- `404` for missing calendar/event
- `409` for conflicts
- `502` when CalDAV is unavailable or returns an invalid upstream response

## Conflict Handling

For writes:

- if etag/revision is available, use it
- if stale update is detected, return conflict
- do not overwrite blindly if safe concurrency checks are possible

## Timezone Rules

- all stored/returned timestamps must be explicit ISO 8601
- timezone handling must be consistent
- default timezone should be configurable
- do not assume floating local times unless CalDAV data explicitly requires it

## Logging

Log:

- request id
- method
- path
- action result
- upstream CalDAV errors

Do not log secrets.

## Security

V1 can bind only to localhost.

Recommended:

- bind to `127.0.0.1`
- keep CalDAV credentials server-side only
- if exposed later, add token auth or mTLS

## Deployment

Deliverable should be a single runnable binary.

Expected run mode:

- long-running local service
- suitable for runit

Provide:

- example env file
- README
- runit service example

## Testing

Must include tests for:

- event normalization
- input validation
- time range validation
- availability calculation
- CalDAV error mapping

## Acceptance Criteria

V1 is acceptable when:

- it can list upcoming events from CalDAV
- it can create a new event in the configured calendar
- it can move an event
- it can delete an event
- it can return free/busy info
- it returns clean JSON only
- it does not require touching raw CalDAV from the agent side
- it is runnable as one local service

## Implementation Advice

- keep internal modules small
- make CalDAV interaction a clean adapter layer
- keep HTTP handlers thin
- keep normalization logic separate from transport
- do not overbuild abstractions

## Suggested Layout

```txt
cmd/calendar-api/main.go
internal/api/
internal/caldav/
internal/events/
internal/availability/
internal/config/
README.md
.env.example
runit/
```

## Naming

Service name:

- `calendar-api`

Binary name:

- `calendar-api`
