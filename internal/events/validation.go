package events

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var ErrInvalidTimeRange = errors.New("invalid time range")

const (
	TodoStatusNeedsAction = "NEEDS-ACTION"
	TodoStatusInProcess   = "IN-PROCESS"
	TodoStatusCompleted   = "COMPLETED"
	TodoStatusCancelled   = "CANCELLED"
)

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

func ValidateCreateTodo(req CreateTodoRequest, defaultCalendar string, defaultLoc *time.Location) (TodoInput, error) {
	start, err := ParseOptionalTimestamp(req.Start)
	if err != nil {
		return TodoInput{}, fmt.Errorf("invalid start: %w", err)
	}
	due, err := ParseOptionalTimestamp(req.Due)
	if err != nil {
		return TodoInput{}, fmt.Errorf("invalid due: %w", err)
	}
	completed, err := ParseOptionalTimestamp(req.Completed)
	if err != nil {
		return TodoInput{}, fmt.Errorf("invalid completed: %w", err)
	}

	input := TodoInput{
		Calendar:    firstNonEmpty(req.Calendar, defaultCalendar),
		Title:       strings.TrimSpace(req.Title),
		Description: req.Description,
		Start:       start,
		Due:         due,
		Completed:   completed,
		AllDay:      req.AllDay,
		Timezone:    strings.TrimSpace(req.Timezone),
		Status:      strings.TrimSpace(req.Status),
	}
	if req.PercentComplete != nil {
		input.PercentComplete = *req.PercentComplete
	}

	return normalizeTodoInput(input, defaultLoc)
}

func ValidatePatchTodo(existing Todo, req PatchTodoRequest, defaultLoc *time.Location) (TodoInput, error) {
	input := TodoInput{
		Calendar:        existing.Calendar,
		Title:           existing.Title,
		Description:     existing.Description,
		Start:           cloneTimePtr(existing.Start),
		Due:             cloneTimePtr(existing.Due),
		Completed:       cloneTimePtr(existing.Completed),
		AllDay:          existing.AllDay,
		Timezone:        existing.Timezone,
		Status:          existing.Status,
		PercentComplete: existing.PercentComplete,
	}
	var err error

	if req.Title != nil {
		input.Title = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		input.Description = *req.Description
	}
	if req.Start != nil {
		input.Start, err = parsePatchOptionalTimestamp(*req.Start)
		if err != nil {
			return TodoInput{}, fmt.Errorf("invalid start: %w", err)
		}
	}
	if req.Due != nil {
		input.Due, err = parsePatchOptionalTimestamp(*req.Due)
		if err != nil {
			return TodoInput{}, fmt.Errorf("invalid due: %w", err)
		}
	}
	if req.Completed != nil {
		input.Completed, err = parsePatchOptionalTimestamp(*req.Completed)
		if err != nil {
			return TodoInput{}, fmt.Errorf("invalid completed: %w", err)
		}
	}
	if req.AllDay != nil {
		input.AllDay = *req.AllDay
	}
	if req.Timezone != nil {
		input.Timezone = strings.TrimSpace(*req.Timezone)
	}
	if req.Status != nil {
		input.Status = strings.TrimSpace(*req.Status)
	}
	if req.PercentComplete != nil {
		input.PercentComplete = *req.PercentComplete
	}

	return normalizeTodoInput(input, defaultLoc)
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

func ParseOptionalTimestamp(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	ts, err := ParseTimestamp(raw)
	if err != nil {
		return nil, err
	}
	return &ts, nil
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

func normalizeTodoInput(input TodoInput, defaultLoc *time.Location) (TodoInput, error) {
	if input.Calendar == "" {
		return TodoInput{}, errors.New("calendar is required")
	}
	if input.Title == "" {
		return TodoInput{}, errors.New("title is required")
	}

	loc := defaultLoc
	if strings.TrimSpace(input.Timezone) != "" {
		var err error
		loc, err = time.LoadLocation(strings.TrimSpace(input.Timezone))
		if err != nil {
			return TodoInput{}, errors.New("invalid timezone")
		}
	} else {
		input.Timezone = defaultLoc.String()
	}

	input.Start = normalizeTodoTime(input.Start, loc)
	input.Due = normalizeTodoTime(input.Due, loc)
	input.Completed = normalizeTodoTime(input.Completed, loc)

	if input.AllDay {
		for _, value := range []*time.Time{input.Start, input.Due, input.Completed} {
			if value != nil && !midnight(*value) {
				return TodoInput{}, errors.New("all-day todos must use midnight boundaries")
			}
		}
	}
	if input.Start != nil && input.Due != nil && input.Due.Before(*input.Start) {
		return TodoInput{}, ErrInvalidTimeRange
	}

	status, err := normalizeTodoStatus(input.Status)
	if err != nil {
		return TodoInput{}, err
	}
	input.Status = status
	if input.PercentComplete < 0 || input.PercentComplete > 100 {
		return TodoInput{}, errors.New("invalid percentComplete")
	}
	if input.Completed != nil {
		input.Status = TodoStatusCompleted
		if input.PercentComplete < 100 {
			input.PercentComplete = 100
		}
	}
	if input.Status == TodoStatusCompleted && input.PercentComplete < 100 {
		input.PercentComplete = 100
	}
	if input.Timezone == "" {
		input.Timezone = loc.String()
	}
	return input, nil
}

func FinalizeTodo(input TodoInput, now time.Time) TodoInput {
	if input.Status != TodoStatusCompleted {
		input.Completed = nil
		return input
	}
	if input.PercentComplete < 100 {
		input.PercentComplete = 100
	}
	if input.Completed != nil {
		return input
	}

	loc := now.Location()
	if strings.TrimSpace(input.Timezone) != "" {
		if loaded, err := time.LoadLocation(strings.TrimSpace(input.Timezone)); err == nil {
			loc = loaded
		}
	}
	completed := now.In(loc)
	if input.AllDay {
		completed = time.Date(completed.Year(), completed.Month(), completed.Day(), 0, 0, 0, 0, loc)
	}
	input.Completed = &completed
	return input
}

func midnight(t time.Time) bool {
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}

func normalizeTodoStatus(raw string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", TodoStatusNeedsAction:
		return TodoStatusNeedsAction, nil
	case TodoStatusInProcess:
		return TodoStatusInProcess, nil
	case TodoStatusCompleted:
		return TodoStatusCompleted, nil
	case TodoStatusCancelled:
		return TodoStatusCancelled, nil
	default:
		return "", errors.New("invalid status")
	}
}

func normalizeTodoTime(value *time.Time, loc *time.Location) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.In(loc)
	return &normalized
}

func parsePatchOptionalTimestamp(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	return ParseOptionalTimestamp(raw)
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
