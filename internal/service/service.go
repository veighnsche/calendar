package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"calendar-api/internal/availability"
	"calendar-api/internal/caldav"
	"calendar-api/internal/config"
	"calendar-api/internal/events"
)

var slugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

type Service struct {
	client          *caldav.Client
	logger          *slog.Logger
	defaultCalendar string
	defaultLoc      *time.Location
}

type ListEventsParams struct {
	Calendar string
	From     string
	To       string
	Limit    int
	Query    string
}

type UpcomingEventsParams struct {
	Calendar string
	Limit    int
}

type GetEventParams struct {
	Calendar string
	ID       string
}

type PatchEventParams struct {
	Calendar string
	ID       string
	Body     events.PatchRequest
}

type MoveEventParams struct {
	Calendar string
	ID       string
	Body     events.MoveRequest
}

type DeleteEventParams struct {
	Calendar string
	ID       string
	ETag     string
	DryRun   bool
}

type AvailabilityParams struct {
	Calendar        string
	From            string
	To              string
	DurationMinutes int
}

type HealthResult struct {
	Status string         `json:"status"`
	CalDAV HealthUpstream `json:"caldav"`
}

type HealthUpstream struct {
	Reachable      bool   `json:"reachable"`
	UserCollection string `json:"userCollection,omitempty"`
	Error          string `json:"error,omitempty"`
}

type EventResult struct {
	DryRun bool         `json:"dryRun"`
	Event  events.Event `json:"event"`
}

type DeleteResult struct {
	DryRun   bool   `json:"dryRun"`
	Deleted  bool   `json:"deleted"`
	ID       string `json:"id"`
	Calendar string `json:"calendar"`
}

type AvailabilityResult struct {
	Calendar        string            `json:"calendar"`
	From            time.Time         `json:"from"`
	To              time.Time         `json:"to"`
	Busy            []events.Interval `json:"busy"`
	Free            []events.Interval `json:"free,omitempty"`
	DurationMinutes int               `json:"durationMinutes"`
}

func New(cfg config.Config, client *caldav.Client, logger *slog.Logger) (*Service, error) {
	loc, err := cfg.DefaultLocation()
	if err != nil {
		return nil, err
	}
	return &Service{
		client:          client,
		logger:          logger,
		defaultCalendar: cfg.DefaultCalendar,
		defaultLoc:      loc,
	}, nil
}

func (s *Service) Health(ctx context.Context) (HealthResult, error) {
	status, err := s.client.Health(ctx)
	if err != nil {
		return HealthResult{
			Status: "degraded",
			CalDAV: HealthUpstream{
				Reachable: false,
				Error:     PublicError(err),
			},
		}, err
	}
	return HealthResult{
		Status: "ok",
		CalDAV: HealthUpstream{
			Reachable:      status.Reachable,
			UserCollection: status.UserCollection,
		},
	}, nil
}

func (s *Service) ListCalendars(ctx context.Context) ([]events.Calendar, error) {
	return s.client.ListCalendars(ctx)
}

func (s *Service) ListEvents(ctx context.Context, params ListEventsParams) ([]events.Event, error) {
	limit, err := clampLimit(params.Limit, 100, 500)
	if err != nil {
		return nil, err
	}

	var from, to *time.Time
	if strings.TrimSpace(params.From) != "" || strings.TrimSpace(params.To) != "" {
		parsedFrom, parsedTo, err := events.ParseRange(params.From, params.To)
		if err != nil {
			return nil, err
		}
		from = &parsedFrom
		to = &parsedTo
	}

	return s.client.ListEvents(ctx, caldav.ListOptions{
		Calendar: s.resolveCalendar(params.Calendar),
		From:     from,
		To:       to,
		Limit:    limit,
		Query:    params.Query,
		Expand:   from != nil && to != nil,
	})
}

func (s *Service) UpcomingEvents(ctx context.Context, params UpcomingEventsParams) ([]events.Event, error) {
	limit, err := clampLimit(params.Limit, 10, 100)
	if err != nil {
		return nil, err
	}
	return s.client.UpcomingEvents(ctx, s.resolveCalendar(params.Calendar), limit)
}

func (s *Service) GetEvent(ctx context.Context, params GetEventParams) (events.Event, error) {
	object, err := s.client.GetObject(ctx, s.resolveCalendar(params.Calendar), params.ID)
	if err != nil {
		return events.Event{}, err
	}
	return object.Event, nil
}

func (s *Service) CreateEvent(ctx context.Context, req events.CreateRequest) (EventResult, error) {
	input, err := events.ValidateCreate(req, s.defaultCalendar, s.defaultLoc)
	if err != nil {
		return EventResult{}, err
	}
	id, err := newObjectID(input.Title)
	if err != nil {
		return EventResult{}, err
	}
	data, err := events.BuildCalendar(id, input, time.Now())
	if err != nil {
		return EventResult{}, err
	}
	if req.DryRun {
		event, err := events.NormalizeSingleEvent(input.Calendar, id, data, "", s.defaultLoc)
		if err != nil {
			return EventResult{}, err
		}
		return EventResult{DryRun: true, Event: event}, nil
	}

	item, err := s.client.PutObject(ctx, input.Calendar, id, data, "", true)
	if err != nil {
		return EventResult{}, err
	}
	return EventResult{DryRun: false, Event: item.Event}, nil
}

func (s *Service) PatchEvent(ctx context.Context, params PatchEventParams) (EventResult, error) {
	calendar := s.resolveCalendar(params.Calendar)
	object, err := s.client.GetObject(ctx, calendar, params.ID)
	if err != nil {
		return EventResult{}, err
	}

	input, err := events.ValidatePatch(object.Event, params.Body, s.defaultLoc)
	if err != nil {
		return EventResult{}, err
	}
	data, err := events.PatchCalendar(object.Data, input, time.Now())
	if err != nil {
		return EventResult{}, err
	}

	if params.Body.DryRun {
		event, err := events.NormalizeSingleEvent(calendar, object.ID, data, object.ETag, s.defaultLoc)
		if err != nil {
			return EventResult{}, err
		}
		return EventResult{DryRun: true, Event: event}, nil
	}

	etag := strings.TrimSpace(resolveWriteETag("", params.Body.ETag))
	if etag == "" {
		return EventResult{}, errors.New("missing etag")
	}
	item, err := s.client.PutObject(ctx, calendar, object.ID, data, etag, false)
	if err != nil {
		return EventResult{}, err
	}
	return EventResult{DryRun: false, Event: item.Event}, nil
}

func (s *Service) PatchEventWithETag(ctx context.Context, params PatchEventParams, etag string) (EventResult, error) {
	params.Body.ETag = stringPtr(firstNonEmpty(etag, valueOrEmpty(params.Body.ETag)))
	return s.PatchEvent(ctx, params)
}

func (s *Service) MoveEvent(ctx context.Context, params MoveEventParams) (EventResult, error) {
	calendar := s.resolveCalendar(params.Calendar)
	object, err := s.client.GetObject(ctx, calendar, params.ID)
	if err != nil {
		return EventResult{}, err
	}

	input, targetCalendar, err := events.ValidateMove(object.Event, params.Body, s.defaultLoc)
	if err != nil {
		return EventResult{}, err
	}
	data, err := events.PatchCalendar(object.Data, input, time.Now())
	if err != nil {
		return EventResult{}, err
	}

	if params.Body.DryRun {
		event, err := events.NormalizeSingleEvent(targetCalendar, object.ID, data, object.ETag, s.defaultLoc)
		if err != nil {
			return EventResult{}, err
		}
		return EventResult{DryRun: true, Event: event}, nil
	}

	etag := strings.TrimSpace(resolveWriteETag("", params.Body.ETag))
	if etag == "" {
		return EventResult{}, errors.New("missing etag")
	}

	if targetCalendar == calendar {
		item, err := s.client.PutObject(ctx, calendar, object.ID, data, etag, false)
		if err != nil {
			return EventResult{}, err
		}
		return EventResult{DryRun: false, Event: item.Event}, nil
	}

	item, err := s.client.PutObject(ctx, targetCalendar, object.ID, data, "", true)
	if err != nil {
		return EventResult{}, err
	}
	if err := s.client.DeleteObject(ctx, calendar, object.ID, etag); err != nil {
		if rollbackErr := s.client.DeleteObject(ctx, targetCalendar, object.ID, item.ETag); rollbackErr != nil {
			s.logger.Error("move rollback failed", "calendar", targetCalendar, "id", object.ID, "error", rollbackErr)
		}
		return EventResult{}, err
	}
	return EventResult{DryRun: false, Event: item.Event}, nil
}

func (s *Service) MoveEventWithETag(ctx context.Context, params MoveEventParams, etag string) (EventResult, error) {
	params.Body.ETag = stringPtr(firstNonEmpty(etag, valueOrEmpty(params.Body.ETag)))
	return s.MoveEvent(ctx, params)
}

func (s *Service) DeleteEvent(ctx context.Context, params DeleteEventParams) (DeleteResult, error) {
	calendar := s.resolveCalendar(params.Calendar)
	if params.DryRun {
		if _, err := s.client.GetObject(ctx, calendar, params.ID); err != nil {
			return DeleteResult{}, err
		}
		return DeleteResult{
			DryRun:   true,
			Deleted:  true,
			ID:       params.ID,
			Calendar: calendar,
		}, nil
	}

	if strings.TrimSpace(params.ETag) == "" {
		return DeleteResult{}, errors.New("missing etag")
	}
	if err := s.client.DeleteObject(ctx, calendar, params.ID, strings.TrimSpace(params.ETag)); err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{
		DryRun:   false,
		Deleted:  true,
		ID:       params.ID,
		Calendar: calendar,
	}, nil
}

func (s *Service) Availability(ctx context.Context, params AvailabilityParams) (AvailabilityResult, error) {
	from, to, err := events.ParseRange(params.From, params.To)
	if err != nil {
		return AvailabilityResult{}, err
	}
	duration, hasDuration, err := parseDurationMinutes(params.DurationMinutes)
	if err != nil {
		return AvailabilityResult{}, err
	}

	calendar := s.resolveCalendar(params.Calendar)
	items, err := s.client.ListEvents(ctx, caldav.ListOptions{
		Calendar: calendar,
		From:     &from,
		To:       &to,
		Expand:   true,
	})
	if err != nil {
		return AvailabilityResult{}, err
	}

	busy := availability.BusyIntervals(items, from, to)
	var free []events.Interval
	if hasDuration {
		free = availability.FreeSlots(busy, from, to, duration)
	}
	return AvailabilityResult{
		Calendar:        calendar,
		From:            from,
		To:              to,
		Busy:            busy,
		Free:            free,
		DurationMinutes: params.DurationMinutes,
	}, nil
}

func PublicError(err error) string {
	if err == nil {
		return ""
	}

	switch {
	case errors.Is(err, caldav.ErrCalendarNotFound):
		return "calendar not found"
	case errors.Is(err, caldav.ErrEventNotFound):
		return "event not found"
	case errors.Is(err, caldav.ErrWriteConflict):
		return "write conflict"
	case errors.Is(err, caldav.ErrCalDAVUnavailable):
		return "caldav unavailable"
	case errors.Is(err, events.ErrInvalidTimeRange):
		return "invalid time range"
	case strings.Contains(strings.ToLower(err.Error()), "internal server"):
		return "internal server error"
	default:
		return err.Error()
	}
}

func HTTPStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, caldav.ErrCalendarNotFound), errors.Is(err, caldav.ErrEventNotFound):
		return http.StatusNotFound
	case errors.Is(err, caldav.ErrWriteConflict):
		return http.StatusConflict
	case errors.Is(err, caldav.ErrCalDAVUnavailable):
		return http.StatusBadGateway
	case strings.Contains(strings.ToLower(err.Error()), "internal server"):
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

func resolveWriteETag(header string, body *string) string {
	if value := strings.TrimSpace(header); value != "" {
		return value
	}
	if body == nil {
		return ""
	}
	return strings.TrimSpace(*body)
}

func (s *Service) resolveCalendar(raw string) string {
	if value := strings.TrimSpace(raw); value != "" {
		return value
	}
	return s.defaultCalendar
}

func clampLimit(limit, defaultValue, max int) (int, error) {
	if limit == 0 {
		return defaultValue, nil
	}
	if limit < 0 {
		return 0, errors.New("invalid limit")
	}
	if limit > max {
		return max, nil
	}
	return limit, nil
}

func parseDurationMinutes(minutes int) (time.Duration, bool, error) {
	if minutes == 0 {
		return 0, false, nil
	}
	if minutes < 0 {
		return 0, false, errors.New("invalid duration_minutes")
	}
	return time.Duration(minutes) * time.Minute, true, nil
}

func newObjectID(title string) (string, error) {
	base := strings.ToLower(strings.TrimSpace(title))
	base = slugCleaner.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "event"
	}

	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate event id: %w", err)
	}
	return base + "-" + hex.EncodeToString(buf), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	v := strings.TrimSpace(value)
	return &v
}
