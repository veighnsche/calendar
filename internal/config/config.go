package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	RadicaleBaseURL  string
	RadicaleUsername string
	RadicalePassword string
	DefaultCalendar  string
	BindAddr         string
	DefaultTimezone  string
}

func Load() (Config, error) {
	cfg := Config{
		RadicaleBaseURL:  strings.TrimSpace(os.Getenv("RADICALE_BASE_URL")),
		RadicaleUsername: strings.TrimSpace(os.Getenv("RADICALE_USERNAME")),
		RadicalePassword: os.Getenv("RADICALE_PASSWORD"),
		DefaultCalendar:  strings.TrimSpace(os.Getenv("CALENDAR_DEFAULT_NAME")),
		BindAddr:         strings.TrimSpace(os.Getenv("API_BIND_ADDR")),
		DefaultTimezone:  strings.TrimSpace(os.Getenv("DEFAULT_TIMEZONE")),
	}

	if cfg.DefaultTimezone == "" {
		cfg.DefaultTimezone = "UTC"
	}

	var missing []string
	if cfg.RadicaleBaseURL == "" {
		missing = append(missing, "RADICALE_BASE_URL")
	}
	if cfg.RadicaleUsername == "" {
		missing = append(missing, "RADICALE_USERNAME")
	}
	if cfg.RadicalePassword == "" {
		missing = append(missing, "RADICALE_PASSWORD")
	}
	if cfg.DefaultCalendar == "" {
		missing = append(missing, "CALENDAR_DEFAULT_NAME")
	}
	if cfg.BindAddr == "" {
		missing = append(missing, "API_BIND_ADDR")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}

	if _, err := url.ParseRequestURI(cfg.RadicaleBaseURL); err != nil {
		return Config{}, fmt.Errorf("invalid RADICALE_BASE_URL: %w", err)
	}
	if _, err := time.LoadLocation(cfg.DefaultTimezone); err != nil {
		return Config{}, fmt.Errorf("invalid DEFAULT_TIMEZONE: %w", err)
	}
	if err := validateBindAddr(cfg.BindAddr); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) DefaultLocation() (*time.Location, error) {
	return time.LoadLocation(c.DefaultTimezone)
}

func validateBindAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid API_BIND_ADDR: %w", err)
	}
	host = strings.TrimSpace(host)
	host = strings.Trim(host, "[]")
	if host == "" {
		return errors.New("API_BIND_ADDR must bind to localhost")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("API_BIND_ADDR must bind to localhost")
	}
	return nil
}
