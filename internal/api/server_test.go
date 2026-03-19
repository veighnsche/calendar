package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"calendar-api/internal/caldav"
	"calendar-api/internal/config"
	"calendar-api/internal/service"
)

func TestCalDAVErrorsMapToBadGateway(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		CalDAVBaseURL:   upstream.URL,
		CalDAVUsername:  "vince",
		CalDAVPassword:  "secret",
		DefaultCalendar: "wall",
		BindAddr:        "127.0.0.1:8090",
		DefaultTimezone: "UTC",
	}
	client, err := caldav.NewClient(cfg, logger)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	app, err := service.New(cfg, client, logger)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	server := NewServer(app, logger)

	req := httptest.NewRequest(http.MethodGet, "/calendars", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "caldav unavailable" {
		t.Fatalf("unexpected error body: %#v", body)
	}
}
