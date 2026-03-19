package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"time"

	"calendar-api/internal/bootstrap"
	"calendar-api/internal/config"
	"calendar-api/internal/mcpserver"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{}))
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

	_, app, err := bootstrap.Build(startupCtx, input, logger)
	if err != nil {
		logger.Error("bootstrap calendar-api-mcp", "error", err)
		os.Exit(1)
	}

	server := mcpserver.New(app, logger)
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		logger.Error("mcp server exited", "error", err)
		os.Exit(1)
	}
}
