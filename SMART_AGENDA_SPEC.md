# Smart Agenda Logic Spec

## Purpose

This document defines the higher-level agenda logic that should live in the calendar service layer, above raw CalDAV storage.

The CalDAV server stores calendar truth.

The calendar service is responsible for smart behavior such as:

- where the user needs to be
- when the user must leave
- whether the user should walk, bike, drive, or take transit
- when reminders should fire
- when a preparation reminder should appear
- when an event should appear as "start getting ready now"

This logic must not live in the wall frontend.
This logic must not live inside the upstream CalDAV server.
This logic should not be buried only inside an MCP adapter.

It belongs in the calendar service at:

- `/home/vince/Projects/calendar`

## Scope

This spec is about smart agenda behavior and derived planning logic.

It is separate from:

- raw event CRUD
- raw CalDAV access
- basic API transport

## Core Principle

The calendar service should expose both:

1. raw calendar events
2. derived actionable agenda items

Example:

- raw event: "Dentist", `2026-03-26T14:00`, location Rotterdam
- derived agenda:
  - leave home at `12:58`
  - reminder at `12:45`
  - preparation starts at `12:30`

## Why This Exists

CalDAV/iCalendar can store:

- title
- time
- location
- alarms

But it does not provide your full practical planning behavior out of the box.

The service must therefore compute those practical fields.

## Event Enrichment Model

The service should support extra planning metadata per event.

Suggested fields:

- `location_text`
- `location_lat`
- `location_lon`
- `travel_mode`
- `travel_buffer_minutes`
- `arrival_buffer_minutes`
- `preparation_minutes`
- `reminder_minutes_before`
- `home_profile`
- `travel_required`

These may come from:

- explicit event fields
- event description parsing
- API-level metadata storage
- defaults from user preferences

## User Profile Defaults

The service should support user-level defaults.

Examples:

- default home address
- preferred walking speed
- preferred transit usage
- standard arrival buffer
- standard preparation buffer
- default reminder offsets

Suggested config:

- `HOME_ADDRESS`
- `HOME_LAT`
- `HOME_LON`
- `DEFAULT_TRAVEL_MODE`
- `DEFAULT_ARRIVAL_BUFFER_MINUTES`
- `DEFAULT_PREPARATION_MINUTES`
- `DEFAULT_REMINDER_MINUTES_BEFORE`

## Derived Fields

For an event with time and location, the service should be able to derive:

- `travel_duration_minutes`
- `leave_at`
- `prepare_at`
- `remind_at`
- `arrival_target_at`
- `travel_required`
- `travel_plan_status`

Example derived shape:

```json
{
  "event_id": "dentist-2026-03-26",
  "title": "Tandarts",
  "start": "2026-03-26T14:00:00+01:00",
  "location_text": "Rotterdam Centrum",
  "travel_mode": "transit",
  "travel_duration_minutes": 47,
  "arrival_buffer_minutes": 10,
  "preparation_minutes": 15,
  "leave_at": "2026-03-26T13:03:00+01:00",
  "prepare_at": "2026-03-26T12:48:00+01:00",
  "remind_at": "2026-03-26T12:45:00+01:00",
  "travel_required": true,
  "travel_plan_status": "ready"
}
```

## Travel Modes

Supported in v1:

- `walk`
- `bike`
- `drive`
- `transit`
- `none`

Behavior:

- `none` means no travel planning
- if no mode is specified, use user default
- if the event has no location, travel planning may be skipped

## Reminder Types

The service should support multiple reminder concepts.

Suggested reminder types:

- event reminder
- preparation reminder
- leave-now reminder
- final-call reminder

These may map back into:

- computed API fields
- optional generated `VALARM`s in CalDAV

## Reminder Logic

Suggested ordering:

1. event start time is fixed
2. arrival buffer is subtracted
3. travel duration is subtracted
4. preparation buffer is subtracted
5. configured reminder offsets are applied

This creates:

- target arrival time
- leave time
- preparation start
- reminder times

## Availability / Planning Logic

The service should also support:

- checking whether a new event conflicts with existing travel and preparation windows
- determining free slots
- finding a slot that includes travel feasibility

Important:

A free slot is not just calendar-free time.
It is only valid if it also leaves enough time for preparation and travel.

## API Additions

The base calendar API should expose higher-level endpoints such as:

- `GET /agenda/upcoming`
- `GET /agenda/today`
- `GET /agenda/:id/plan`
- `POST /agenda/plan`
- `POST /events/:id/recompute`

### `GET /agenda/upcoming`

Returns enriched upcoming agenda items for display and agents.

### `GET /agenda/:id/plan`

Returns computed planning details for one event.

### `POST /agenda/plan`

Input:

- title
- start
- end
- location
- travel mode
- user preferences

Output:

- derived planning result
- warnings
- reminder schedule

## Wall-Focused Output

Because the wall is a major consumer, the service should expose a wall-friendly agenda view.

Suggested wall shape:

```json
{
  "items": [
    {
      "id": "gemeente-bezoek",
      "date_iso": "2026-03-24",
      "date_label": "dinsdag 24 maart",
      "time": "12:30",
      "title": "Gemeente bezoek",
      "context": "Afspraak bij de gemeente.",
      "location": "Stadhuis",
      "travel_mode": "walk",
      "leave_at": "12:05",
      "prepare_at": "11:55",
      "status": "leave-soon"
    }
  ]
}
```

## Agent-Focused Output

The service should make agent usage simple.

Example agent questions:

- "When do I need to leave for the dentist?"
- "Can I still fit groceries in before my train?"
- "Move this appointment to tomorrow afternoon with enough travel time."
- "Add a reminder 20 minutes before I need to leave."

The service should answer these without forcing the agent to understand CalDAV internals.

## Source Of Truth Strategy

Raw event truth remains in CalDAV.

Derived planning data may be:

- computed on demand
- cached in memory
- optionally persisted later if needed

Do not create a second hidden source of truth in v1.

## Geocoding / Routing

V1 may start without live routing APIs.

Recommended staged approach:

### V1

- explicit travel mode
- explicit manually configured travel duration
- static buffers

### V2

- geocoding
- route lookup
- transit estimation
- traffic-aware updates

### V3

- dynamic “leave now” recalculation
- train/traffic disruption awareness

## V1 Recommendation

Do not block the project on live routing.

For v1, support:

- location text
- travel mode
- travel duration override
- leave time computation
- preparation time
- reminders

That already gives agents meaningful planning power.

## Data Model Additions For V1

Suggested event metadata additions:

- `travel_mode`
- `travel_duration_minutes`
- `arrival_buffer_minutes`
- `preparation_minutes`
- `location_text`

This is enough to compute:

- leave time
- prepare time
- reminders

without needing maps APIs yet.

## Error Handling

Smart agenda logic must fail explicitly.

Examples:

- `location missing`
- `travel duration missing`
- `cannot compute leave time`
- `invalid timezone`
- `event already started`

## Safety Rules

- never silently invent travel durations
- never silently overwrite existing reminders
- never silently move event start/end times while only computing travel logic
- distinguish between raw event time and derived leave/prep times

## UI Implications

The frontend should render enriched fields, but not compute them.

Examples of frontend-only usage:

- show `leave at 12:05`
- show `walk in 25 min`
- show `start getting ready`
- highlight days with agenda items

These are outputs of the service, not frontend business rules.

## MCP Position

If an MCP server is added later, it should call the calendar API.

The MCP layer should not be the primary place where planning logic lives.

Reason:

- multiple consumers need the same behavior
- MCP is an interface layer, not the business-logic core

## Acceptance Criteria

This smart agenda layer is acceptable when:

- an event can express where the user needs to be
- the service can compute when the user must leave
- the service can compute preparation time
- the service can compute one or more reminder moments
- the service can expose these derived values over JSON
- the wall can consume those derived values without recomputing them
- an AI agent can use the service without knowing CalDAV internals

## Suggested Next Step

Build this inside the `calendar` project as a separate module alongside the base calendar API:

- raw CalDAV adapter
- event CRUD
- smart planning layer
- wall-friendly agenda projection
