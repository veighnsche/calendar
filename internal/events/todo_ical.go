package events

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

var ErrTodoNotFound = errors.New("todo not found")

func NormalizeTodoCalendarObject(calendarName, objectID string, data []byte, etag string, defaultLoc *time.Location) ([]Todo, error) {
	cal, err := ics.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse calendar object: %w", err)
	}

	result := make([]Todo, 0, len(cal.Todos()))
	for _, vtodo := range cal.Todos() {
		todo, ok, err := normalizeVTodo(calendarName, objectID, etag, vtodo, defaultLoc)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, todo)
		}
	}
	if len(result) == 0 {
		return nil, ErrTodoNotFound
	}
	return result, nil
}

func NormalizeSingleTodo(calendarName, objectID string, data []byte, etag string, defaultLoc *time.Location) (Todo, error) {
	todos, err := NormalizeTodoCalendarObject(calendarName, objectID, data, etag, defaultLoc)
	if err != nil {
		return Todo{}, err
	}
	return pickPrimaryTodo(todos), nil
}

func BuildTodoCalendar(id string, input TodoInput, now time.Time) ([]byte, error) {
	cal := ics.NewCalendar()
	cal.SetProductId("-//calendar-api//self-hosted calendar adapter//EN")
	cal.SetXWRCalName(input.Calendar)
	cal.SetXWRTimezone(input.Timezone)

	todo := cal.AddTodo(id)
	applyComponentMetadata(&todo.ComponentBase, now)
	setTextProperty(&todo.ComponentBase, ics.ComponentPropertySummary, input.Title)
	setTextProperty(&todo.ComponentBase, ics.ComponentPropertyDescription, input.Description)
	if err := setTodoWindow(&todo.ComponentBase, input); err != nil {
		return nil, err
	}
	applyTodoState(&todo.ComponentBase, input)

	return []byte(cal.Serialize()), nil
}

func PatchTodoCalendar(data []byte, input TodoInput, now time.Time) ([]byte, error) {
	cal, err := ics.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse calendar object: %w", err)
	}

	todo := primaryTodo(cal)
	if todo == nil {
		return nil, ErrTodoNotFound
	}

	applyComponentMetadata(&todo.ComponentBase, now)
	setTextProperty(&todo.ComponentBase, ics.ComponentPropertySummary, input.Title)
	setTextProperty(&todo.ComponentBase, ics.ComponentPropertyDescription, input.Description)
	if err := setTodoWindow(&todo.ComponentBase, input); err != nil {
		return nil, err
	}
	applyTodoState(&todo.ComponentBase, input)

	return []byte(cal.Serialize()), nil
}

func normalizeVTodo(calendarName, objectID, etag string, vtodo *ics.VTodo, defaultLoc *time.Location) (Todo, bool, error) {
	status := strings.ToUpper(strings.TrimSpace(propertyValue(vtodo.ComponentBase.GetProperty(ics.ComponentPropertyStatus))))
	if status == TodoStatusCancelled {
		return Todo{}, false, nil
	}

	var (
		start     *time.Time
		due       *time.Time
		completed *time.Time
		allDay    bool
		timezone  string
		err       error
	)

	if startProp := vtodo.ComponentBase.GetProperty(ics.ComponentPropertyDtStart); startProp != nil {
		startValue, startAllDay, startTimezone, parseErr := parseICalTime(startProp, defaultLoc)
		if parseErr != nil {
			return Todo{}, false, parseErr
		}
		start = &startValue
		allDay = startAllDay
		timezone = startTimezone
	}
	if dueProp := vtodo.ComponentBase.GetProperty(ics.ComponentPropertyDue); dueProp != nil {
		dueValue, dueAllDay, dueTimezone, parseErr := parseICalTime(dueProp, defaultLoc)
		if parseErr != nil {
			return Todo{}, false, parseErr
		}
		due = &dueValue
		if dueAllDay {
			allDay = true
		}
		if timezone == "" {
			timezone = dueTimezone
		}
	}
	if completedProp := vtodo.ComponentBase.GetProperty(ics.ComponentPropertyCompleted); completedProp != nil {
		completedValue, completedAllDay, completedTimezone, parseErr := parseICalTime(completedProp, defaultLoc)
		if parseErr != nil {
			return Todo{}, false, parseErr
		}
		completed = &completedValue
		if completedAllDay {
			allDay = true
		}
		if timezone == "" {
			timezone = completedTimezone
		}
	}
	if timezone == "" {
		timezone = defaultLoc.String()
	}

	percentComplete := 0
	if prop := vtodo.ComponentBase.GetProperty(ics.ComponentPropertyPercentComplete); prop != nil {
		percentComplete, err = strconv.Atoi(strings.TrimSpace(prop.Value))
		if err != nil {
			return Todo{}, false, errors.New("todo has invalid PERCENT-COMPLETE")
		}
	}
	if status == "" {
		status = TodoStatusNeedsAction
	}

	return Todo{
		ID:              objectID,
		Calendar:        calendarName,
		Title:           propertyValue(vtodo.ComponentBase.GetProperty(ics.ComponentPropertySummary)),
		Description:     propertyValue(vtodo.ComponentBase.GetProperty(ics.ComponentPropertyDescription)),
		Start:           start,
		Due:             due,
		Completed:       completed,
		AllDay:          allDay,
		Timezone:        timezone,
		Status:          status,
		PercentComplete: percentComplete,
		ETag:            etag,
		Source:          SourceCalDAV,
	}, true, nil
}

func setTodoWindow(todo *ics.ComponentBase, input TodoInput) error {
	loc, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return errors.New("invalid timezone")
	}

	if err := setOptionalTimeProperty(todo, ics.ComponentPropertyDtStart, input.Start, input.AllDay, loc); err != nil {
		return err
	}
	if err := setOptionalTimeProperty(todo, ics.ComponentPropertyDue, input.Due, input.AllDay, loc); err != nil {
		return err
	}
	if err := setOptionalTimeProperty(todo, ics.ComponentPropertyCompleted, input.Completed, input.AllDay, loc); err != nil {
		return err
	}
	return nil
}

func setOptionalTimeProperty(component *ics.ComponentBase, prop ics.ComponentProperty, value *time.Time, allDay bool, loc *time.Location) error {
	component.RemoveProperty(prop)
	if value == nil {
		return nil
	}
	if loc == nil {
		return errors.New("invalid timezone")
	}
	if allDay {
		component.SetProperty(prop, value.In(loc).Format("20060102"), ics.WithValue(string(ics.ValueDataTypeDate)))
		return nil
	}
	component.SetProperty(prop, value.In(loc).Format("20060102T150405"), ics.WithTZID(loc.String()))
	return nil
}

func applyTodoState(todo *ics.ComponentBase, input TodoInput) {
	todo.RemoveProperty(ics.ComponentPropertyStatus)
	todo.RemoveProperty(ics.ComponentPropertyPercentComplete)

	if input.Status != "" {
		todo.SetProperty(ics.ComponentPropertyStatus, input.Status)
	}
	todo.SetProperty(ics.ComponentPropertyPercentComplete, strconv.Itoa(input.PercentComplete))
}

func primaryTodo(cal *ics.Calendar) *ics.VTodo {
	todos := cal.Todos()
	if len(todos) == 0 {
		return nil
	}
	return todos[0]
}

func pickPrimaryTodo(values []Todo) Todo {
	if len(values) == 0 {
		return Todo{}
	}

	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Completed == nil && values[j].Completed != nil {
			return true
		}
		if values[i].Completed != nil && values[j].Completed == nil {
			return false
		}
		if earlierTodoTime(values[i].Due, values[j].Due) {
			return true
		}
		if earlierTodoTime(values[j].Due, values[i].Due) {
			return false
		}
		if earlierTodoTime(values[i].Start, values[j].Start) {
			return true
		}
		if earlierTodoTime(values[j].Start, values[i].Start) {
			return false
		}
		return values[i].Title < values[j].Title
	})
	return values[0]
}

func earlierTodoTime(left, right *time.Time) bool {
	if left == nil {
		return false
	}
	if right == nil {
		return true
	}
	return left.Before(*right)
}
