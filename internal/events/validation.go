package events

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidTimeRange = errors.New("invalid time range")

func ValidateCreate(req CreateRequest, defaultCalendar string, defaultLoc *time.Location) (EventInput, error) {
	start, err := ParseTimestamp(req.Start)
	if err != nil {
		return EventInput{}, fmt.Errorf("invalid start: %w", err)
	}
	end, err := ParseTimestamp(req.End)
	if err != nil {
		return EventInput{}, fmt.Errorf("invalid end: %w", err)
	}

	input := EventInput{
		Calendar:    firstNonEmpty(req.Calendar, defaultCalendar),
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Start:       start,
		End:         end,
		AllDay:      req.AllDay,
		Timezone:    strings.TrimSpace(req.Timezone),
		Location:    req.Location,
	}

	return normalizeInput(input, defaultLoc)
}

func ValidatePatch(existing Event, req PatchRequest, defaultLoc *time.Location) (EventInput, error) {
	input := EventInput{
		Calendar:    existing.Calendar,
		Title:       existing.Title,
		Description: existing.Description,
		Start:       existing.Start,
		End:         existing.End,
		AllDay:      existing.AllDay,
		Timezone:    existing.Timezone,
		Location:    existing.Location,
	}

	if req.Title != nil {
		input.Title = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		input.Description = *req.Description
	}
	if req.Start != nil {
		start, err := ParseTimestamp(*req.Start)
		if err != nil {
			return EventInput{}, fmt.Errorf("invalid start: %w", err)
		}
		input.Start = start
	}
	if req.End != nil {
		end, err := ParseTimestamp(*req.End)
		if err != nil {
			return EventInput{}, fmt.Errorf("invalid end: %w", err)
		}
		input.End = end
	}
	if req.AllDay != nil {
		input.AllDay = *req.AllDay
	}
	if req.Timezone != nil {
		input.Timezone = strings.TrimSpace(*req.Timezone)
	}
	if req.Location != nil {
		input.Location = *req.Location
	}

	return normalizeInput(input, defaultLoc)
}

func ValidateMove(existing Event, req MoveRequest, defaultLoc *time.Location) (EventInput, string, error) {
	start, err := ParseTimestamp(req.Start)
	if err != nil {
		return EventInput{}, "", fmt.Errorf("invalid start: %w", err)
	}
	end, err := ParseTimestamp(req.End)
	if err != nil {
		return EventInput{}, "", fmt.Errorf("invalid end: %w", err)
	}

	input := EventInput{
		Calendar:    existing.Calendar,
		Title:       existing.Title,
		Description: existing.Description,
		Start:       start,
		End:         end,
		AllDay:      existing.AllDay,
		Timezone:    existing.Timezone,
		Location:    existing.Location,
	}
	input, err = normalizeInput(input, defaultLoc)
	if err != nil {
		return EventInput{}, "", err
	}

	targetCalendar := existing.Calendar
	if req.Calendar != nil && strings.TrimSpace(*req.Calendar) != "" {
		targetCalendar = strings.TrimSpace(*req.Calendar)
	}
	return input, targetCalendar, nil
}

func ParseTimestamp(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("timestamp is required")
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errors.New("timestamp must be RFC3339")
	}
	return ts, nil
}

func ParseRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	from, err := ParseTimestamp(fromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid from: %w", err)
	}
	to, err := ParseTimestamp(toRaw)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid to: %w", err)
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, ErrInvalidTimeRange
	}
	return from, to, nil
}

func ParseLimit(raw string, defaultValue, max int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, errors.New("invalid limit")
	}
	if limit > max {
		return max, nil
	}
	return limit, nil
}

func ParseDurationMinutes(raw string) (time.Duration, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, false, nil
	}
	minutes, err := strconv.Atoi(raw)
	if err != nil || minutes <= 0 {
		return 0, false, errors.New("invalid duration_minutes")
	}
	return time.Duration(minutes) * time.Minute, true, nil
}

func normalizeInput(input EventInput, defaultLoc *time.Location) (EventInput, error) {
	if input.Calendar == "" {
		return EventInput{}, errors.New("calendar is required")
	}
	if input.Title == "" {
		return EventInput{}, errors.New("title is required")
	}
	if !input.End.After(input.Start) {
		return EventInput{}, ErrInvalidTimeRange
	}

	loc := defaultLoc
	if strings.TrimSpace(input.Timezone) != "" {
		var err error
		loc, err = time.LoadLocation(strings.TrimSpace(input.Timezone))
		if err != nil {
			return EventInput{}, errors.New("invalid timezone")
		}
	} else {
		input.Timezone = defaultLoc.String()
	}

	if input.AllDay {
		startDate := input.Start.In(loc)
		endDate := input.End.In(loc)
		if !midnight(startDate) || !midnight(endDate) {
			return EventInput{}, errors.New("all-day events must use midnight boundaries")
		}
	}

	input.Start = input.Start.In(loc)
	input.End = input.End.In(loc)
	if input.Timezone == "" {
		input.Timezone = loc.String()
	}
	return input, nil
}

func midnight(t time.Time) bool {
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
