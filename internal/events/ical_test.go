package events

import (
	"testing"
	"time"
)

func TestNormalizeSingleEvent(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	input := EventInput{
		Calendar:    "wall",
		Title:       "Gemeente bezoek",
		Description: "Afspraak bij de gemeente.",
		Start:       time.Date(2026, 3, 24, 12, 30, 0, 0, loc),
		End:         time.Date(2026, 3, 24, 13, 0, 0, 0, loc),
		Timezone:    "Europe/Paris",
		Location:    "Town hall",
	}
	data, err := BuildCalendar("gemeente-bezoek", input, time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build calendar: %v", err)
	}

	got, err := NormalizeSingleEvent("wall", "gemeente-bezoek", data, `"abc123"`, loc)
	if err != nil {
		t.Fatalf("normalize event: %v", err)
	}

	if got.ID != "gemeente-bezoek" {
		t.Fatalf("unexpected id: %q", got.ID)
	}
	if got.Calendar != "wall" {
		t.Fatalf("unexpected calendar: %q", got.Calendar)
	}
	if got.Title != input.Title {
		t.Fatalf("unexpected title: %q", got.Title)
	}
	if got.Description != input.Description {
		t.Fatalf("unexpected description: %q", got.Description)
	}
	if !got.Start.Equal(input.Start) {
		t.Fatalf("unexpected start: %s", got.Start)
	}
	if !got.End.Equal(input.End) {
		t.Fatalf("unexpected end: %s", got.End)
	}
	if got.AllDay {
		t.Fatal("expected timed event")
	}
	if got.Timezone != "Europe/Paris" {
		t.Fatalf("unexpected timezone: %q", got.Timezone)
	}
	if got.Location != "Town hall" {
		t.Fatalf("unexpected location: %q", got.Location)
	}
	if got.ETag != `"abc123"` {
		t.Fatalf("unexpected etag: %q", got.ETag)
	}
	if got.Source != SourceRadicale {
		t.Fatalf("unexpected source: %q", got.Source)
	}
}
