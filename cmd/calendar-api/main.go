package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"calendar-api/internal/api"
	"calendar-api/internal/bootstrap"
	"calendar-api/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{}))
	input := config.EnvInput()
	flag.StringVar(&input.CalDAVBaseURL, "caldav-base-url", input.CalDAVBaseURL, "CalDAV base URL")
	flag.StringVar(&input.CalDAVUsername, "caldav-username", input.CalDAVUsername, "CalDAV username")
	flag.StringVar(&input.CalDAVPassword, "caldav-password", input.CalDAVPassword, "CalDAV password")
	flag.StringVar(&input.DefaultCalendar, "calendar-default-name", input.DefaultCalendar, "Default calendar name")
	flag.StringVar(&input.BindAddr, "api-bind-addr", input.BindAddr, "HTTP bind address")
	flag.StringVar(&input.DefaultTimezone, "default-timezone", input.DefaultTimezone, "Default timezone")
	flag.Parse()

	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStartup()

	cfg, app, err := bootstrap.Build(startupCtx, input, logger)
	if err != nil {
		logger.Error("bootstrap calendar-api", "error", err)
		os.Exit(1)
	}
	server := api.NewServer(app, logger)

	httpServer := &http.Server{
		Addr:              cfg.BindAddr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Info("starting calendar-api", "bind_addr", cfg.BindAddr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}
