package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"calendar-api/internal/config"
	"calendar-api/internal/radicale"
)

func TestRadicaleErrorsMapToBadGateway(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer upstream.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := config.Config{
		RadicaleBaseURL:  upstream.URL,
		RadicaleUsername: "vince",
		RadicalePassword: "secret",
		DefaultCalendar:  "wall",
		BindAddr:         "127.0.0.1:8090",
		DefaultTimezone:  "UTC",
	}
	client, err := radicale.NewClient(cfg, logger)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	server, err := NewServer(cfg, client, logger)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

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
	if body["error"] != "radicale unavailable" {
		t.Fatalf("unexpected error body: %#v", body)
	}
}
