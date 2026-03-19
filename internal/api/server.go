package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"calendar-api/internal/availability"
	"calendar-api/internal/config"
	"calendar-api/internal/events"
	"calendar-api/internal/radicale"
)

type Server struct {
	client          *radicale.Client
	logger          *slog.Logger
	defaultCalendar string
	defaultLoc      *time.Location
}

type contextKey string

const requestIDKey contextKey = "request_id"

var slugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

func NewServer(cfg config.Config, client *radicale.Client, logger *slog.Logger) (*Server, error) {
	loc, err := cfg.DefaultLocation()
	if err != nil {
		return nil, err
	}
	return &Server{
		client:          client,
		logger:          logger,
		defaultCalendar: cfg.DefaultCalendar,
		defaultLoc:      loc,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /calendars", s.handleCalendars)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("GET /events/upcoming", s.handleUpcoming)
	mux.HandleFunc("GET /events/{id}", s.handleGetEvent)
	mux.HandleFunc("POST /events", s.handleCreateEvent)
	mux.HandleFunc("PATCH /events/{id}", s.handlePatchEvent)
	mux.HandleFunc("POST /events/{id}/move", s.handleMoveEvent)
	mux.HandleFunc("DELETE /events/{id}", s.handleDeleteEvent)
	mux.HandleFunc("GET /availability", s.handleAvailability)
	return s.requestIDMiddleware(s.loggingMiddleware(mux))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	status, err := s.client.Health(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "degraded",
			"radicale": map[string]any{
				"reachable": false,
				"error":     errorMessage(err),
			},
			"requestId": requestIDFromContext(r.Context()),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"radicale": map[string]any{
			"reachable":      status.Reachable,
			"userCollection": status.UserCollection,
		},
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleCalendars(w http.ResponseWriter, r *http.Request) {
	calendars, err := s.client.ListCalendars(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"calendars": calendars,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	limit, err := events.ParseLimit(r.URL.Query().Get("limit"), 100, 500)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	var from, to *time.Time
	fromRaw := strings.TrimSpace(r.URL.Query().Get("from"))
	toRaw := strings.TrimSpace(r.URL.Query().Get("to"))
	if fromRaw != "" || toRaw != "" {
		parsedFrom, parsedTo, err := events.ParseRange(fromRaw, toRaw)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		from = &parsedFrom
		to = &parsedTo
	}

	items, err := s.client.ListEvents(r.Context(), radicale.ListOptions{
		Calendar: s.resolveCalendar(r.URL.Query().Get("calendar")),
		From:     from,
		To:       to,
		Limit:    limit,
		Query:    r.URL.Query().Get("q"),
		Expand:   from != nil && to != nil,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"events":    items,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleUpcoming(w http.ResponseWriter, r *http.Request) {
	limit, err := events.ParseLimit(r.URL.Query().Get("limit"), 10, 100)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	items, err := s.client.UpcomingEvents(r.Context(), s.resolveCalendar(r.URL.Query().Get("calendar")), limit)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"events":    items,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	item, err := s.client.GetObject(r.Context(), s.resolveCalendar(r.URL.Query().Get("calendar")), r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event":     item.Event,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var req events.CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	input, err := events.ValidateCreate(req, s.defaultCalendar, s.defaultLoc)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	id, err := newObjectID(input.Title)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	data, err := events.BuildCalendar(id, input, time.Now())
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	dryRun := req.DryRun || queryBool(r, "dryRun")
	if dryRun {
		event, err := events.NormalizeSingleEvent(input.Calendar, id, data, "", s.defaultLoc)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"dryRun":    true,
			"event":     event,
			"requestId": requestIDFromContext(r.Context()),
		})
		return
	}

	item, err := s.client.PutObject(r.Context(), input.Calendar, id, data, "", true)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"dryRun":    false,
		"event":     item.Event,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handlePatchEvent(w http.ResponseWriter, r *http.Request) {
	calendar := s.resolveCalendar(r.URL.Query().Get("calendar"))
	object, err := s.client.GetObject(r.Context(), calendar, r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	var req events.PatchRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	input, err := events.ValidatePatch(object.Event, req, s.defaultLoc)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	data, err := events.PatchCalendar(object.Data, input, time.Now())
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	dryRun := req.DryRun || queryBool(r, "dryRun")
	if dryRun {
		event, err := events.NormalizeSingleEvent(calendar, object.ID, data, object.ETag, s.defaultLoc)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"dryRun":    true,
			"event":     event,
			"requestId": requestIDFromContext(r.Context()),
		})
		return
	}

	etag := resolveWriteETag(r.Header.Get("If-Match"), req.ETag)
	if etag == "" {
		s.writeError(w, r, errors.New("missing etag"))
		return
	}

	item, err := s.client.PutObject(r.Context(), calendar, object.ID, data, etag, false)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dryRun":    false,
		"event":     item.Event,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleMoveEvent(w http.ResponseWriter, r *http.Request) {
	calendar := s.resolveCalendar(r.URL.Query().Get("calendar"))
	object, err := s.client.GetObject(r.Context(), calendar, r.PathValue("id"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	var req events.MoveRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	input, targetCalendar, err := events.ValidateMove(object.Event, req, s.defaultLoc)
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	data, err := events.PatchCalendar(object.Data, input, time.Now())
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	dryRun := req.DryRun || queryBool(r, "dryRun")
	if dryRun {
		event, err := events.NormalizeSingleEvent(targetCalendar, object.ID, data, object.ETag, s.defaultLoc)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"dryRun":    true,
			"event":     event,
			"requestId": requestIDFromContext(r.Context()),
		})
		return
	}

	etag := resolveWriteETag(r.Header.Get("If-Match"), req.ETag)
	if etag == "" {
		s.writeError(w, r, errors.New("missing etag"))
		return
	}

	if targetCalendar == calendar {
		item, err := s.client.PutObject(r.Context(), calendar, object.ID, data, etag, false)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"dryRun":    false,
			"event":     item.Event,
			"requestId": requestIDFromContext(r.Context()),
		})
		return
	}

	item, err := s.client.PutObject(r.Context(), targetCalendar, object.ID, data, "", true)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	if err := s.client.DeleteObject(r.Context(), calendar, object.ID, etag); err != nil {
		if rollbackErr := s.client.DeleteObject(r.Context(), targetCalendar, object.ID, item.ETag); rollbackErr != nil {
			s.logger.Error("move rollback failed", "calendar", targetCalendar, "id", object.ID, "error", rollbackErr)
		}
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dryRun":    false,
		"event":     item.Event,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	calendar := s.resolveCalendar(r.URL.Query().Get("calendar"))
	id := r.PathValue("id")
	dryRun := queryBool(r, "dryRun")
	if dryRun {
		if _, err := s.client.GetObject(r.Context(), calendar, id); err != nil {
			s.writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"dryRun":    true,
			"deleted":   true,
			"id":        id,
			"calendar":  calendar,
			"requestId": requestIDFromContext(r.Context()),
		})
		return
	}

	etag := strings.TrimSpace(r.Header.Get("If-Match"))
	if etag == "" {
		etag = strings.TrimSpace(r.URL.Query().Get("etag"))
	}
	if etag == "" {
		s.writeError(w, r, errors.New("missing etag"))
		return
	}

	if err := s.client.DeleteObject(r.Context(), calendar, id, etag); err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dryRun":    false,
		"deleted":   true,
		"id":        id,
		"calendar":  calendar,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleAvailability(w http.ResponseWriter, r *http.Request) {
	from, to, err := events.ParseRange(r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	duration, hasDuration, err := events.ParseDurationMinutes(r.URL.Query().Get("duration_minutes"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	calendar := s.resolveCalendar(r.URL.Query().Get("calendar"))
	items, err := s.client.ListEvents(r.Context(), radicale.ListOptions{
		Calendar: calendar,
		From:     &from,
		To:       &to,
		Expand:   true,
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	busy := availability.BusyIntervals(items, from, to)
	free := []events.Interval(nil)
	if hasDuration {
		free = availability.FreeSlots(busy, from, to, duration)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"calendar":        calendar,
		"from":            from,
		"to":              to,
		"busy":            busy,
		"free":            free,
		"durationMinutes": int(duration / time.Minute),
		"requestId":       requestIDFromContext(r.Context()),
	})
}

func (s *Server) resolveCalendar(raw string) string {
	if value := strings.TrimSpace(raw); value != "" {
		return value
	}
	return s.defaultCalendar
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	message := errorMessage(err)

	switch {
	case errors.Is(err, radicale.ErrCalendarNotFound):
		status = http.StatusNotFound
		message = "calendar not found"
	case errors.Is(err, radicale.ErrEventNotFound):
		status = http.StatusNotFound
		message = "event not found"
	case errors.Is(err, radicale.ErrWriteConflict):
		status = http.StatusConflict
		message = "write conflict"
	case errors.Is(err, radicale.ErrRadicaleUnavailable):
		status = http.StatusBadGateway
		message = "radicale unavailable"
	case errors.Is(err, events.ErrInvalidTimeRange):
		status = http.StatusBadRequest
		message = "invalid time range"
	case strings.Contains(strings.ToLower(err.Error()), "internal server"):
		status = http.StatusInternalServerError
		message = "internal server error"
	}

	writeJSON(w, status, map[string]any{
		"error":     message,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get("X-Request-Id"))
		if requestID == "" {
			requestID = randomToken(8)
		}
		ctx := context.WithValue(r.Context(), requestIDKey, requestID)
		w.Header().Set("X-Request-Id", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(recorder, r)
		s.logger.Info("request completed",
			"request_id", requestIDFromContext(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", recorder.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON body")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid JSON body")
	}
	return nil
}

func requestIDFromContext(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey).(string)
	return requestID
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

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func queryBool(r *http.Request, key string) bool {
	value := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(key)))
	return value == "1" || value == "true" || value == "yes"
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

func randomToken(size int) string {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
