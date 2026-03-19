package availability

import (
	"testing"
	"time"

	"calendar-api/internal/events"
)

func TestBusyIntervalsAndFreeSlots(t *testing.T) {
	loc := time.UTC
	from := time.Date(2026, 3, 24, 9, 0, 0, 0, loc)
	to := time.Date(2026, 3, 24, 15, 0, 0, 0, loc)
	items := []events.Event{
		{Start: time.Date(2026, 3, 24, 10, 0, 0, 0, loc), End: time.Date(2026, 3, 24, 11, 0, 0, 0, loc)},
		{Start: time.Date(2026, 3, 24, 10, 30, 0, 0, loc), End: time.Date(2026, 3, 24, 12, 0, 0, 0, loc)},
		{Start: time.Date(2026, 3, 24, 13, 0, 0, 0, loc), End: time.Date(2026, 3, 24, 14, 0, 0, 0, loc)},
	}

	busy := BusyIntervals(items, from, to)
	if len(busy) != 2 {
		t.Fatalf("expected 2 busy intervals, got %d", len(busy))
	}
	if !busy[0].Start.Equal(time.Date(2026, 3, 24, 10, 0, 0, 0, loc)) || !busy[0].End.Equal(time.Date(2026, 3, 24, 12, 0, 0, 0, loc)) {
		t.Fatalf("unexpected first busy interval: %+v", busy[0])
	}
	if !busy[1].Start.Equal(time.Date(2026, 3, 24, 13, 0, 0, 0, loc)) || !busy[1].End.Equal(time.Date(2026, 3, 24, 14, 0, 0, 0, loc)) {
		t.Fatalf("unexpected second busy interval: %+v", busy[1])
	}

	free := FreeSlots(busy, from, to, 30*time.Minute)
	if len(free) != 3 {
		t.Fatalf("expected 3 free slots, got %d", len(free))
	}
	if !free[0].Start.Equal(from) || !free[0].End.Equal(time.Date(2026, 3, 24, 10, 0, 0, 0, loc)) {
		t.Fatalf("unexpected first free slot: %+v", free[0])
	}
	if !free[1].Start.Equal(time.Date(2026, 3, 24, 12, 0, 0, 0, loc)) || !free[1].End.Equal(time.Date(2026, 3, 24, 13, 0, 0, 0, loc)) {
		t.Fatalf("unexpected second free slot: %+v", free[1])
	}
	if !free[2].Start.Equal(time.Date(2026, 3, 24, 14, 0, 0, 0, loc)) || !free[2].End.Equal(to) {
		t.Fatalf("unexpected third free slot: %+v", free[2])
	}
}
