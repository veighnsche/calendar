package availability

import (
	"sort"
	"time"

	"calendar-api/internal/events"
)

func BusyIntervals(items []events.Event, from, to time.Time) []events.Interval {
	busy := make([]events.Interval, 0, len(items))
	for _, item := range items {
		start := maxTime(item.Start, from)
		end := minTime(item.End, to)
		if !end.After(start) {
			continue
		}
		busy = append(busy, events.Interval{Start: start, End: end})
	}
	if len(busy) == 0 {
		return nil
	}
	sort.Slice(busy, func(i, j int) bool {
		if busy[i].Start.Equal(busy[j].Start) {
			return busy[i].End.Before(busy[j].End)
		}
		return busy[i].Start.Before(busy[j].Start)
	})

	merged := []events.Interval{busy[0]}
	for _, interval := range busy[1:] {
		last := &merged[len(merged)-1]
		if interval.Start.After(last.End) {
			merged = append(merged, interval)
			continue
		}
		if interval.End.After(last.End) {
			last.End = interval.End
		}
	}
	return merged
}

func FreeSlots(busy []events.Interval, from, to time.Time, minDuration time.Duration) []events.Interval {
	if !to.After(from) {
		return nil
	}
	if len(busy) == 0 {
		if minDuration == 0 || to.Sub(from) >= minDuration {
			return []events.Interval{{Start: from, End: to}}
		}
		return nil
	}

	free := make([]events.Interval, 0, len(busy)+1)
	cursor := from
	for _, interval := range busy {
		if interval.Start.After(cursor) {
			slot := events.Interval{Start: cursor, End: interval.Start}
			if minDuration == 0 || slot.End.Sub(slot.Start) >= minDuration {
				free = append(free, slot)
			}
		}
		if interval.End.After(cursor) {
			cursor = interval.End
		}
	}
	if to.After(cursor) {
		slot := events.Interval{Start: cursor, End: to}
		if minDuration == 0 || slot.End.Sub(slot.Start) >= minDuration {
			free = append(free, slot)
		}
	}
	return free
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
