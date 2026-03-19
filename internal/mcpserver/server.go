package mcpserver

import (
	"context"
	"errors"
	"log/slog"

	"calendar-api/internal/events"
	"calendar-api/internal/service"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type calendarService interface {
	Health(context.Context) (service.HealthResult, error)
	ListCalendars(context.Context) ([]events.Calendar, error)
	ListEvents(context.Context, service.ListEventsParams) ([]events.Event, error)
	UpcomingEvents(context.Context, service.UpcomingEventsParams) ([]events.Event, error)
	GetEvent(context.Context, service.GetEventParams) (events.Event, error)
	CreateEvent(context.Context, events.CreateRequest) (service.EventResult, error)
	PatchEventWithETag(context.Context, service.PatchEventParams, string) (service.EventResult, error)
	MoveEventWithETag(context.Context, service.MoveEventParams, string) (service.EventResult, error)
	DeleteEvent(context.Context, service.DeleteEventParams) (service.DeleteResult, error)
	Availability(context.Context, service.AvailabilityParams) (service.AvailabilityResult, error)
}

type handler struct {
	service calendarService
}

type emptyInput struct{}

type listCalendarsOutput struct {
	Calendars []events.Calendar `json:"calendars"`
}

type listEventsInput struct {
	Calendar string `json:"calendar,omitempty" jsonschema:"calendar name; defaults to the configured default calendar"`
	From     string `json:"from,omitempty" jsonschema:"range start in RFC3339 format"`
	To       string `json:"to,omitempty" jsonschema:"range end in RFC3339 format"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of events to return"`
	Query    string `json:"query,omitempty" jsonschema:"case-insensitive text filter for title, description, location, or id"`
}

type listEventsOutput struct {
	Events []events.Event `json:"events"`
}

type upcomingEventsInput struct {
	Calendar string `json:"calendar,omitempty" jsonschema:"calendar name; defaults to the configured default calendar"`
	Limit    int    `json:"limit,omitempty" jsonschema:"maximum number of upcoming events to return"`
}

type getEventInput struct {
	Calendar string `json:"calendar,omitempty" jsonschema:"calendar name; defaults to the configured default calendar"`
	ID       string `json:"id" jsonschema:"event identifier"`
}

type getEventOutput struct {
	Event events.Event `json:"event"`
}

type createEventInput struct {
	Calendar    string `json:"calendar,omitempty" jsonschema:"target calendar; defaults to the configured default calendar"`
	Title       string `json:"title" jsonschema:"event title"`
	Description string `json:"description,omitempty" jsonschema:"event description"`
	Start       string `json:"start" jsonschema:"event start time in RFC3339 format"`
	End         string `json:"end" jsonschema:"event end time in RFC3339 format"`
	AllDay      bool   `json:"allDay,omitempty" jsonschema:"whether the event is an all-day event"`
	Timezone    string `json:"timezone,omitempty" jsonschema:"IANA timezone name such as Europe/Paris"`
	Location    string `json:"location,omitempty" jsonschema:"event location"`
	DryRun      bool   `json:"dryRun,omitempty" jsonschema:"validate and preview without writing to CalDAV"`
}

type updateEventInput struct {
	Calendar    string  `json:"calendar,omitempty" jsonschema:"calendar name; defaults to the configured default calendar"`
	ID          string  `json:"id" jsonschema:"event identifier"`
	Title       *string `json:"title,omitempty" jsonschema:"new event title"`
	Description *string `json:"description,omitempty" jsonschema:"new event description"`
	Start       *string `json:"start,omitempty" jsonschema:"new event start time in RFC3339 format"`
	End         *string `json:"end,omitempty" jsonschema:"new event end time in RFC3339 format"`
	AllDay      *bool   `json:"allDay,omitempty" jsonschema:"whether the event is an all-day event"`
	Timezone    *string `json:"timezone,omitempty" jsonschema:"IANA timezone name such as Europe/Paris"`
	Location    *string `json:"location,omitempty" jsonschema:"new event location"`
	ETag        string  `json:"etag,omitempty" jsonschema:"current event etag for optimistic concurrency; fetch it with get_event before updating"`
	DryRun      bool    `json:"dryRun,omitempty" jsonschema:"validate and preview without writing to CalDAV"`
}

type moveEventInput struct {
	Calendar       string `json:"calendar,omitempty" jsonschema:"current calendar name; defaults to the configured default calendar"`
	ID             string `json:"id" jsonschema:"event identifier"`
	Start          string `json:"start" jsonschema:"new start time in RFC3339 format"`
	End            string `json:"end" jsonschema:"new end time in RFC3339 format"`
	TargetCalendar string `json:"targetCalendar,omitempty" jsonschema:"optional destination calendar name"`
	ETag           string `json:"etag,omitempty" jsonschema:"current event etag for optimistic concurrency; fetch it with get_event before moving"`
	DryRun         bool   `json:"dryRun,omitempty" jsonschema:"validate and preview without writing to CalDAV"`
}

type deleteEventInput struct {
	Calendar string `json:"calendar,omitempty" jsonschema:"calendar name; defaults to the configured default calendar"`
	ID       string `json:"id" jsonschema:"event identifier"`
	ETag     string `json:"etag,omitempty" jsonschema:"current event etag for optimistic concurrency; fetch it with get_event before deleting"`
	DryRun   bool   `json:"dryRun,omitempty" jsonschema:"validate and preview without deleting the event"`
}

type availabilityInput struct {
	Calendar        string `json:"calendar,omitempty" jsonschema:"calendar name; defaults to the configured default calendar"`
	From            string `json:"from" jsonschema:"range start in RFC3339 format"`
	To              string `json:"to" jsonschema:"range end in RFC3339 format"`
	DurationMinutes int    `json:"durationMinutes,omitempty" jsonschema:"minimum free slot size in minutes"`
}

func New(service calendarService, logger *slog.Logger) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "calendar-api",
		Version: "v1.0.0",
	}, &mcp.ServerOptions{
		Logger:       logger,
		Instructions: "Manage the user's CalDAV calendar safely. Use get_event to retrieve the current etag before update_event, move_event, or delete_event. All timestamps must be RFC3339.",
	})

	h := &handler{service: service}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "health",
		Description: "Check whether the calendar service can reach the configured CalDAV server.",
	}, h.health)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_calendars",
		Description: "List calendar collections visible to the configured user.",
	}, h.listCalendars)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_events",
		Description: "List normalized calendar events, optionally filtered by time range and text query.",
	}, h.listEvents)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_upcoming_events",
		Description: "List the next upcoming events from now.",
	}, h.listUpcomingEvents)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_event",
		Description: "Fetch a single event, including its current etag.",
	}, h.getEvent)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "create_event",
		Description: "Create a new event in a calendar. Supports dry-run preview mode.",
	}, h.createEvent)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "update_event",
		Description: "Update an existing event. Requires the current etag unless dryRun is true.",
	}, h.updateEvent)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "move_event",
		Description: "Move an event to a new time, optionally into another calendar. Requires the current etag unless dryRun is true.",
	}, h.moveEvent)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete_event",
		Description: "Delete an event. Requires the current etag unless dryRun is true.",
	}, h.deleteEvent)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_availability",
		Description: "Return busy intervals and optional free slots for a time range.",
	}, h.getAvailability)

	return server
}

func (h *handler) health(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, service.HealthResult, error) {
	result, _ := h.service.Health(ctx)
	return nil, result, nil
}

func (h *handler) listCalendars(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, listCalendarsOutput, error) {
	calendars, err := h.service.ListCalendars(ctx)
	if err != nil {
		return nil, listCalendarsOutput{}, toolError(err)
	}
	return nil, listCalendarsOutput{Calendars: calendars}, nil
}

func (h *handler) listEvents(ctx context.Context, _ *mcp.CallToolRequest, input listEventsInput) (*mcp.CallToolResult, listEventsOutput, error) {
	items, err := h.service.ListEvents(ctx, service.ListEventsParams{
		Calendar: input.Calendar,
		From:     input.From,
		To:       input.To,
		Limit:    input.Limit,
		Query:    input.Query,
	})
	if err != nil {
		return nil, listEventsOutput{}, toolError(err)
	}
	return nil, listEventsOutput{Events: items}, nil
}

func (h *handler) listUpcomingEvents(ctx context.Context, _ *mcp.CallToolRequest, input upcomingEventsInput) (*mcp.CallToolResult, listEventsOutput, error) {
	items, err := h.service.UpcomingEvents(ctx, service.UpcomingEventsParams{
		Calendar: input.Calendar,
		Limit:    input.Limit,
	})
	if err != nil {
		return nil, listEventsOutput{}, toolError(err)
	}
	return nil, listEventsOutput{Events: items}, nil
}

func (h *handler) getEvent(ctx context.Context, _ *mcp.CallToolRequest, input getEventInput) (*mcp.CallToolResult, getEventOutput, error) {
	event, err := h.service.GetEvent(ctx, service.GetEventParams{
		Calendar: input.Calendar,
		ID:       input.ID,
	})
	if err != nil {
		return nil, getEventOutput{}, toolError(err)
	}
	return nil, getEventOutput{Event: event}, nil
}

func (h *handler) createEvent(ctx context.Context, _ *mcp.CallToolRequest, input createEventInput) (*mcp.CallToolResult, service.EventResult, error) {
	result, err := h.service.CreateEvent(ctx, events.CreateRequest{
		Calendar:    input.Calendar,
		Title:       input.Title,
		Description: input.Description,
		Start:       input.Start,
		End:         input.End,
		AllDay:      input.AllDay,
		Timezone:    input.Timezone,
		Location:    input.Location,
		DryRun:      input.DryRun,
	})
	if err != nil {
		return nil, service.EventResult{}, toolError(err)
	}
	return nil, result, nil
}

func (h *handler) updateEvent(ctx context.Context, _ *mcp.CallToolRequest, input updateEventInput) (*mcp.CallToolResult, service.EventResult, error) {
	result, err := h.service.PatchEventWithETag(ctx, service.PatchEventParams{
		Calendar: input.Calendar,
		ID:       input.ID,
		Body: events.PatchRequest{
			Title:       input.Title,
			Description: input.Description,
			Start:       input.Start,
			End:         input.End,
			AllDay:      input.AllDay,
			Timezone:    input.Timezone,
			Location:    input.Location,
			DryRun:      input.DryRun,
		},
	}, input.ETag)
	if err != nil {
		return nil, service.EventResult{}, toolError(err)
	}
	return nil, result, nil
}

func (h *handler) moveEvent(ctx context.Context, _ *mcp.CallToolRequest, input moveEventInput) (*mcp.CallToolResult, service.EventResult, error) {
	var target *string
	if input.TargetCalendar != "" {
		target = &input.TargetCalendar
	}
	result, err := h.service.MoveEventWithETag(ctx, service.MoveEventParams{
		Calendar: input.Calendar,
		ID:       input.ID,
		Body: events.MoveRequest{
			Start:    input.Start,
			End:      input.End,
			Calendar: target,
			DryRun:   input.DryRun,
		},
	}, input.ETag)
	if err != nil {
		return nil, service.EventResult{}, toolError(err)
	}
	return nil, result, nil
}

func (h *handler) deleteEvent(ctx context.Context, _ *mcp.CallToolRequest, input deleteEventInput) (*mcp.CallToolResult, service.DeleteResult, error) {
	result, err := h.service.DeleteEvent(ctx, service.DeleteEventParams{
		Calendar: input.Calendar,
		ID:       input.ID,
		ETag:     input.ETag,
		DryRun:   input.DryRun,
	})
	if err != nil {
		return nil, service.DeleteResult{}, toolError(err)
	}
	return nil, result, nil
}

func (h *handler) getAvailability(ctx context.Context, _ *mcp.CallToolRequest, input availabilityInput) (*mcp.CallToolResult, service.AvailabilityResult, error) {
	result, err := h.service.Availability(ctx, service.AvailabilityParams{
		Calendar:        input.Calendar,
		From:            input.From,
		To:              input.To,
		DurationMinutes: input.DurationMinutes,
	})
	if err != nil {
		return nil, service.AvailabilityResult{}, toolError(err)
	}
	return nil, result, nil
}

func toolError(err error) error {
	return errors.New(service.PublicError(err))
}
