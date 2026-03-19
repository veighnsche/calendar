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
	"strings"
	"time"

	"calendar-api/internal/events"
	"calendar-api/internal/service"
)

type Server struct {
	service *service.Service
	logger  *slog.Logger
}

type contextKey string

const requestIDKey contextKey = "request_id"

func NewServer(app *service.Service, logger *slog.Logger) *Server {
	return &Server{
		service: app,
		logger:  logger,
	}
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
	mux.HandleFunc("GET /todos", s.handleTodos)
	mux.HandleFunc("GET /todos/{id}", s.handleGetTodo)
	mux.HandleFunc("POST /todos", s.handleCreateTodo)
	mux.HandleFunc("PATCH /todos/{id}", s.handlePatchTodo)
	mux.HandleFunc("DELETE /todos/{id}", s.handleDeleteTodo)
	mux.HandleFunc("GET /availability", s.handleAvailability)
	return s.requestIDMiddleware(s.loggingMiddleware(mux))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	result, err := s.service.Health(r.Context())
	status := http.StatusOK
	if err != nil {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]any{
		"status":    result.Status,
		"caldav":    result.CalDAV,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleCalendars(w http.ResponseWriter, r *http.Request) {
	calendars, err := s.service.ListCalendars(r.Context())
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
	items, err := s.service.ListEvents(r.Context(), service.ListEventsParams{
		Calendar: r.URL.Query().Get("calendar"),
		From:     r.URL.Query().Get("from"),
		To:       r.URL.Query().Get("to"),
		Limit:    limit,
		Query:    r.URL.Query().Get("q"),
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
	items, err := s.service.UpcomingEvents(r.Context(), service.UpcomingEventsParams{
		Calendar: r.URL.Query().Get("calendar"),
		Limit:    limit,
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

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	item, err := s.service.GetEvent(r.Context(), service.GetEventParams{
		Calendar: r.URL.Query().Get("calendar"),
		ID:       r.PathValue("id"),
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"event":     item,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	var req events.CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	req.DryRun = req.DryRun || queryBool(r, "dryRun")
	item, err := s.service.CreateEvent(r.Context(), req)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if item.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{
		"dryRun":    item.DryRun,
		"event":     item.Event,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handlePatchEvent(w http.ResponseWriter, r *http.Request) {
	var req events.PatchRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	req.DryRun = req.DryRun || queryBool(r, "dryRun")
	item, err := s.service.PatchEventWithETag(r.Context(), service.PatchEventParams{
		Calendar: r.URL.Query().Get("calendar"),
		ID:       r.PathValue("id"),
		Body:     req,
	}, r.Header.Get("If-Match"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dryRun":    item.DryRun,
		"event":     item.Event,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleMoveEvent(w http.ResponseWriter, r *http.Request) {
	var req events.MoveRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	req.DryRun = req.DryRun || queryBool(r, "dryRun")
	item, err := s.service.MoveEventWithETag(r.Context(), service.MoveEventParams{
		Calendar: r.URL.Query().Get("calendar"),
		ID:       r.PathValue("id"),
		Body:     req,
	}, r.Header.Get("If-Match"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dryRun":    item.DryRun,
		"event":     item.Event,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	etag := strings.TrimSpace(r.Header.Get("If-Match"))
	if etag == "" {
		etag = strings.TrimSpace(r.URL.Query().Get("etag"))
	}
	result, err := s.service.DeleteEvent(r.Context(), service.DeleteEventParams{
		Calendar: r.URL.Query().Get("calendar"),
		ID:       r.PathValue("id"),
		ETag:     etag,
		DryRun:   queryBool(r, "dryRun"),
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dryRun":    result.DryRun,
		"deleted":   result.Deleted,
		"id":        result.ID,
		"calendar":  result.Calendar,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleTodos(w http.ResponseWriter, r *http.Request) {
	limit, err := events.ParseLimit(r.URL.Query().Get("limit"), 100, 500)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	items, err := s.service.ListTodos(r.Context(), service.ListTodosParams{
		Calendar: r.URL.Query().Get("calendar"),
		From:     r.URL.Query().Get("from"),
		To:       r.URL.Query().Get("to"),
		Limit:    limit,
		Query:    r.URL.Query().Get("q"),
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"todos":     items,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleGetTodo(w http.ResponseWriter, r *http.Request) {
	item, err := s.service.GetTodo(r.Context(), service.GetTodoParams{
		Calendar: r.URL.Query().Get("calendar"),
		ID:       r.PathValue("id"),
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"todo":      item,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleCreateTodo(w http.ResponseWriter, r *http.Request) {
	var req events.CreateTodoRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	req.DryRun = req.DryRun || queryBool(r, "dryRun")
	item, err := s.service.CreateTodo(r.Context(), req)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	status := http.StatusCreated
	if item.DryRun {
		status = http.StatusOK
	}
	writeJSON(w, status, map[string]any{
		"dryRun":    item.DryRun,
		"todo":      item.Todo,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handlePatchTodo(w http.ResponseWriter, r *http.Request) {
	var req events.PatchTodoRequest
	if err := decodeJSON(r, &req); err != nil {
		s.writeError(w, r, err)
		return
	}
	req.DryRun = req.DryRun || queryBool(r, "dryRun")
	item, err := s.service.PatchTodoWithETag(r.Context(), service.PatchTodoParams{
		Calendar: r.URL.Query().Get("calendar"),
		ID:       r.PathValue("id"),
		Body:     req,
	}, r.Header.Get("If-Match"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dryRun":    item.DryRun,
		"todo":      item.Todo,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleDeleteTodo(w http.ResponseWriter, r *http.Request) {
	etag := strings.TrimSpace(r.Header.Get("If-Match"))
	if etag == "" {
		etag = strings.TrimSpace(r.URL.Query().Get("etag"))
	}
	result, err := s.service.DeleteTodo(r.Context(), service.DeleteTodoParams{
		Calendar: r.URL.Query().Get("calendar"),
		ID:       r.PathValue("id"),
		ETag:     etag,
		DryRun:   queryBool(r, "dryRun"),
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dryRun":    result.DryRun,
		"deleted":   result.Deleted,
		"id":        result.ID,
		"calendar":  result.Calendar,
		"requestId": requestIDFromContext(r.Context()),
	})
}

func (s *Server) handleAvailability(w http.ResponseWriter, r *http.Request) {
	duration, _, err := events.ParseDurationMinutes(r.URL.Query().Get("duration_minutes"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	result, err := s.service.Availability(r.Context(), service.AvailabilityParams{
		Calendar:        r.URL.Query().Get("calendar"),
		From:            r.URL.Query().Get("from"),
		To:              r.URL.Query().Get("to"),
		DurationMinutes: int(duration / time.Minute),
	})
	if err != nil {
		s.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"calendar":        result.Calendar,
		"from":            result.From,
		"to":              result.To,
		"busy":            result.Busy,
		"free":            result.Free,
		"durationMinutes": result.DurationMinutes,
		"requestId":       requestIDFromContext(r.Context()),
	})
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	writeJSON(w, service.HTTPStatus(err), map[string]any{
		"error":     service.PublicError(err),
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

func queryBool(r *http.Request, key string) bool {
	value := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(key)))
	return value == "1" || value == "true" || value == "yes"
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
