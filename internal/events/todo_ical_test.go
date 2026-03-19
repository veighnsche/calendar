package events

import (
	"testing"
	"time"
)

func TestNormalizeSingleTodo(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}

	start := time.Date(2026, 4, 10, 9, 0, 0, 0, loc)
	due := time.Date(2026, 4, 15, 18, 0, 0, 0, loc)
	completed := time.Date(2026, 4, 12, 10, 30, 0, 0, loc)
	input := FinalizeTodo(TodoInput{
		Calendar:        "wall",
		Title:           "Submit Income Taxes",
		Description:     "Mailbox submission",
		Start:           &start,
		Due:             &due,
		Completed:       &completed,
		Timezone:        "Europe/Paris",
		Status:          TodoStatusCompleted,
		PercentComplete: 100,
	}, time.Date(2026, 4, 12, 10, 30, 0, 0, loc))

	data, err := BuildTodoCalendar("submit-income-taxes", input, time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build todo calendar: %v", err)
	}

	got, err := NormalizeSingleTodo("wall", "submit-income-taxes", data, `"etag123"`, loc)
	if err != nil {
		t.Fatalf("normalize todo: %v", err)
	}

	if got.ID != "submit-income-taxes" {
		t.Fatalf("unexpected id: %q", got.ID)
	}
	if got.Title != input.Title {
		t.Fatalf("unexpected title: %q", got.Title)
	}
	if got.Description != input.Description {
		t.Fatalf("unexpected description: %q", got.Description)
	}
	if got.Start == nil || !got.Start.Equal(start) {
		t.Fatalf("unexpected start: %#v", got.Start)
	}
	if got.Due == nil || !got.Due.Equal(due) {
		t.Fatalf("unexpected due: %#v", got.Due)
	}
	if got.Completed == nil || !got.Completed.Equal(completed) {
		t.Fatalf("unexpected completed: %#v", got.Completed)
	}
	if got.Status != TodoStatusCompleted {
		t.Fatalf("unexpected status: %q", got.Status)
	}
	if got.PercentComplete != 100 {
		t.Fatalf("unexpected percentComplete: %d", got.PercentComplete)
	}
	if got.Timezone != "Europe/Paris" {
		t.Fatalf("unexpected timezone: %q", got.Timezone)
	}
	if got.ETag != `"etag123"` {
		t.Fatalf("unexpected etag: %q", got.ETag)
	}
	if got.Source != SourceCalDAV {
		t.Fatalf("unexpected source: %q", got.Source)
	}
}
