package bootstrap

import (
	"context"
	"fmt"
	"log/slog"

	"calendar-api/internal/caldav"
	"calendar-api/internal/config"
	"calendar-api/internal/service"
)

func Build(ctx context.Context, input config.Config, logger *slog.Logger) (config.Config, *service.Service, error) {
	cfg, err := config.LoadFrom(input)
	if err != nil {
		return config.Config{}, nil, err
	}

	client, err := caldav.NewClient(cfg, logger)
	if err != nil {
		return config.Config{}, nil, err
	}
	if _, err := client.Health(ctx); err != nil {
		return config.Config{}, nil, fmt.Errorf("CalDAV startup check failed: %w", err)
	}

	app, err := service.New(cfg, client, logger)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, app, nil
}
