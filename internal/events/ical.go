package events

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

const SourceRadicale = "radicale"

func NormalizeCalendarObject(calendarName, objectID string, data []byte, etag string, defaultLoc *time.Location) ([]Event, error) {
	cal, err := ics.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse calendar object: %w", err)
	}

	result := make([]Event, 0, len(cal.Events()))
	for _, vevent := range cal.Events() {
		event, ok, err := normalizeVEvent(calendarName, objectID, etag, vevent, defaultLoc)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, event)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("event not found")
	}
	return result, nil
}

func NormalizeSingleEvent(calendarName, objectID string, data []byte, etag string, defaultLoc *time.Location) (Event, error) {
	events, err := NormalizeCalendarObject(calendarName, objectID, data, etag, defaultLoc)
	if err != nil {
		return Event{}, err
	}
	return pickPrimaryEvent(events), nil
}

func BuildCalendar(id string, input EventInput, now time.Time) ([]byte, error) {
	cal := ics.NewCalendar()
	cal.SetProductId("-//calendar-api//self-hosted calendar adapter//EN")
	cal.SetXWRCalName(input.Calendar)
	cal.SetXWRTimezone(input.Timezone)

	event := cal.AddEvent(id)
	applyMetadata(event, input, now)
	setTextProperty(event, ics.ComponentPropertySummary, input.Title)
	setTextProperty(event, ics.ComponentPropertyDescription, input.Description)
	setTextProperty(event, ics.ComponentPropertyLocation, input.Location)
	if err := setEventWindow(event, input); err != nil {
		return nil, err
	}

	return []byte(cal.Serialize()), nil
}

func PatchCalendar(data []byte, input EventInput, now time.Time) ([]byte, error) {
	cal, err := ics.ParseCalendar(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse calendar object: %w", err)
	}

	event := primaryEvent(cal)
	if event == nil {
		return nil, errors.New("event not found")
	}

	applyMetadata(event, input, now)
	setTextProperty(event, ics.ComponentPropertySummary, input.Title)
	setTextProperty(event, ics.ComponentPropertyDescription, input.Description)
	setTextProperty(event, ics.ComponentPropertyLocation, input.Location)
	if err := setEventWindow(event, input); err != nil {
		return nil, err
	}

	return []byte(cal.Serialize()), nil
}

func normalizeVEvent(calendarName, objectID, etag string, vevent *ics.VEvent, defaultLoc *time.Location) (Event, bool, error) {
	if status := propertyValue(vevent.ComponentBase.GetProperty(ics.ComponentPropertyStatus)); strings.EqualFold(status, string(ics.ObjectStatusCancelled)) {
		return Event{}, false, nil
	}

	startProp := vevent.ComponentBase.GetProperty(ics.ComponentPropertyDtStart)
	if startProp == nil {
		return Event{}, false, errors.New("event missing DTSTART")
	}
	start, allDay, timezone, err := parseICalTime(startProp, defaultLoc)
	if err != nil {
		return Event{}, false, err
	}

	endProp := vevent.ComponentBase.GetProperty(ics.ComponentPropertyDtEnd)
	end, err := resolveEnd(endProp, start, allDay, defaultLoc)
	if err != nil {
		return Event{}, false, err
	}

	if timezone == "" {
		timezone = defaultLoc.String()
	}

	return Event{
		ID:          objectID,
		Calendar:    calendarName,
		Title:       propertyValue(vevent.ComponentBase.GetProperty(ics.ComponentPropertySummary)),
		Description: propertyValue(vevent.ComponentBase.GetProperty(ics.ComponentPropertyDescription)),
		Start:       start,
		End:         end,
		AllDay:      allDay,
		Timezone:    timezone,
		Location:    propertyValue(vevent.ComponentBase.GetProperty(ics.ComponentPropertyLocation)),
		ETag:        etag,
		Source:      SourceRadicale,
	}, true, nil
}

func parseICalTime(prop *ics.IANAProperty, defaultLoc *time.Location) (time.Time, bool, string, error) {
	if prop == nil {
		return time.Time{}, false, "", errors.New("missing time property")
	}
	value := strings.TrimSpace(prop.Value)
	tzid := propertyParam(prop, string(ics.ParameterTzid))
	valueType := prop.GetValueType()
	loc := defaultLoc
	if tzid != "" {
		var err error
		loc, err = time.LoadLocation(tzid)
		if err != nil {
			return time.Time{}, false, "", fmt.Errorf("invalid timezone %q", tzid)
		}
	}

	switch valueType {
	case ics.ValueDataTypeDate:
		t, err := time.ParseInLocation("20060102", value, loc)
		if err != nil {
			return time.Time{}, false, "", err
		}
		return t, true, timezoneName(tzid, loc), nil
	default:
		if strings.HasSuffix(value, "Z") {
			t, err := time.Parse("20060102T150405Z", value)
			if err != nil {
				return time.Time{}, false, "", err
			}
			return t, false, "UTC", nil
		}
		t, err := time.ParseInLocation("20060102T150405", value, loc)
		if err != nil {
			return time.Time{}, false, "", err
		}
		return t, false, timezoneName(tzid, loc), nil
	}
}

func resolveEnd(prop *ics.IANAProperty, start time.Time, allDay bool, defaultLoc *time.Location) (time.Time, error) {
	if prop == nil {
		if allDay {
			return start.Add(24 * time.Hour), nil
		}
		return start, nil
	}
	end, _, _, err := parseICalTime(prop, defaultLoc)
	if err != nil {
		return time.Time{}, err
	}
	return end, nil
}

func setEventWindow(event *ics.VEvent, input EventInput) error {
	loc, err := time.LoadLocation(input.Timezone)
	if err != nil {
		return errors.New("invalid timezone")
	}

	event.RemoveProperty(ics.ComponentPropertyDuration)
	if input.AllDay {
		event.SetAllDayStartAt(input.Start.In(loc))
		event.SetAllDayEndAt(input.End.In(loc))
		return nil
	}

	event.SetProperty(ics.ComponentPropertyDtStart, input.Start.In(loc).Format("20060102T150405"), ics.WithTZID(loc.String()))
	event.SetProperty(ics.ComponentPropertyDtEnd, input.End.In(loc).Format("20060102T150405"), ics.WithTZID(loc.String()))
	return nil
}

func applyMetadata(event *ics.VEvent, input EventInput, now time.Time) {
	if propertyValue(event.ComponentBase.GetProperty(ics.ComponentPropertyCreated)) == "" {
		event.SetCreatedTime(now.UTC())
	}
	event.SetDtStampTime(now.UTC())
	event.SetLastModifiedAt(now.UTC())
	event.SetSequence(nextSequence(event.ComponentBase.GetProperty(ics.ComponentPropertySequence)))
}

func nextSequence(prop *ics.IANAProperty) int {
	if prop == nil {
		return 0
	}
	var current int
	_, _ = fmt.Sscanf(strings.TrimSpace(prop.Value), "%d", &current)
	return current + 1
}

func setTextProperty(event *ics.VEvent, prop ics.ComponentProperty, value string) {
	if value == "" {
		event.RemoveProperty(prop)
		return
	}
	event.SetProperty(prop, value)
}

func primaryEvent(cal *ics.Calendar) *ics.VEvent {
	events := cal.Events()
	if len(events) == 0 {
		return nil
	}
	for _, event := range events {
		if event.ComponentBase.GetProperty(ics.ComponentPropertyRecurrenceId) == nil {
			return event
		}
	}
	return events[0]
}

func pickPrimaryEvent(values []Event) Event {
	if len(values) == 0 {
		return Event{}
	}
	for _, event := range values {
		if !event.AllDay {
			return event
		}
	}
	return values[0]
}

func propertyValue(prop *ics.IANAProperty) string {
	if prop == nil {
		return ""
	}
	return prop.Value
}

func propertyParam(prop *ics.IANAProperty, key string) string {
	if prop == nil {
		return ""
	}
	values, ok := prop.ICalParameters[key]
	if !ok || len(values) == 0 {
		return ""
	}
	return values[0]
}

func timezoneName(explicit string, loc *time.Location) string {
	if explicit != "" {
		return explicit
	}
	if loc == nil {
		return "UTC"
	}
	return loc.String()
}
