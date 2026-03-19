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
	CalDAVBaseURL   string
	CalDAVUsername  string
	CalDAVPassword  string
	DefaultCalendar string
	BindAddr        string
	DefaultTimezone string
}

func EnvInput() Config {
	return Config{
		CalDAVBaseURL:   strings.TrimSpace(os.Getenv("CALDAV_BASE_URL")),
		CalDAVUsername:  strings.TrimSpace(os.Getenv("CALDAV_USERNAME")),
		CalDAVPassword:  os.Getenv("CALDAV_PASSWORD"),
		DefaultCalendar: strings.TrimSpace(os.Getenv("CALENDAR_DEFAULT_NAME")),
		BindAddr:        strings.TrimSpace(os.Getenv("API_BIND_ADDR")),
		DefaultTimezone: strings.TrimSpace(os.Getenv("DEFAULT_TIMEZONE")),
	}
}

func Load() (Config, error) {
	return LoadFrom(EnvInput())
}

func LoadFrom(input Config) (Config, error) {
	cfg := Config{
		CalDAVBaseURL:   strings.TrimSpace(input.CalDAVBaseURL),
		CalDAVUsername:  strings.TrimSpace(input.CalDAVUsername),
		CalDAVPassword:  input.CalDAVPassword,
		DefaultCalendar: strings.TrimSpace(input.DefaultCalendar),
		BindAddr:        strings.TrimSpace(input.BindAddr),
		DefaultTimezone: strings.TrimSpace(input.DefaultTimezone),
	}

	if cfg.DefaultTimezone == "" {
		cfg.DefaultTimezone = "UTC"
	}

	var missing []string
	if cfg.CalDAVBaseURL == "" {
		missing = append(missing, "CALDAV_BASE_URL")
	}
	if cfg.CalDAVUsername == "" {
		missing = append(missing, "CALDAV_USERNAME")
	}
	if cfg.CalDAVPassword == "" {
		missing = append(missing, "CALDAV_PASSWORD")
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

	if _, err := url.ParseRequestURI(cfg.CalDAVBaseURL); err != nil {
		return Config{}, fmt.Errorf("invalid CALDAV_BASE_URL: %w", err)
	}
	if _, err := time.LoadLocation(cfg.DefaultTimezone); err != nil {
		return Config{}, fmt.Errorf("invalid DEFAULT_TIMEZONE: %w", err)
	}
	if err := validateBindAddr(cfg.BindAddr); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
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
