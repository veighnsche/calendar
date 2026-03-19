package events

import (
	"errors"
	"testing"
	"time"
)

func TestValidateCreateRejectsInvalidRange(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	_, err = ValidateCreate(CreateRequest{
		Title: "Broken",
		Start: "2026-03-24T13:00:00+01:00",
		End:   "2026-03-24T12:30:00+01:00",
	}, "wall", loc)
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("expected invalid time range, got %v", err)
	}
}

func TestParseRangeRejectsInvalidWindow(t *testing.T) {
	_, _, err := ParseRange("2026-03-24T13:00:00+01:00", "2026-03-24T13:00:00+01:00")
	if !errors.Is(err, ErrInvalidTimeRange) {
		t.Fatalf("expected invalid time range, got %v", err)
	}
}
