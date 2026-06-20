package caldav

import (
	"bytes"
	"time"

	"github.com/emersion/go-ical"
)

// timeRange is a half-open [Start, End) window used by calendar-query
// time-range filtering. A zero Start or End is treated as unbounded.
type timeRange struct {
	Start time.Time
	End   time.Time
}

// overlaps reports whether the half-open interval [evStart, evEnd) overlaps
// the (possibly unbounded) range. Per RFC 4791 §9.9 the comparison is
// half-open: an event whose DTEND equals the range start does not overlap.
func (tr timeRange) overlaps(evStart, evEnd time.Time) bool {
	if !tr.Start.IsZero() && !evEnd.After(tr.Start) {
		return false
	}
	if !tr.End.IsZero() && !evStart.Before(tr.End) {
		return false
	}
	return true
}

// matchesTimeRange reports whether the iCalendar object in ics contains at
// least one VEVENT that overlaps tr. Recurring VEVENTs (those carrying an
// RRULE) are expanded via rrule-go and each generated instance is tested
// against the range, so recurrence instances inside the window match even
// when the master DTSTART lies outside it.
func matchesTimeRange(ics []byte, tr timeRange) bool {
	cal, err := ical.NewDecoder(bytes.NewReader(ics)).Decode()
	if err != nil {
		return false
	}
	for i := range cal.Children {
		comp := cal.Children[i]
		if comp.Name != ical.CompEvent {
			continue
		}
		if eventOverlaps(comp, tr) {
			return true
		}
	}
	return false
}

// eventDuration returns the duration of a VEVENT, derived from DTSTART/DTEND
// or DURATION, defaulting to zero when neither is usable.
func eventDuration(ev *ical.Component) time.Duration {
	start, err := ev.Props.DateTime(ical.PropDateTimeStart, time.UTC)
	if err != nil {
		return 0
	}
	if endProp := ev.Props.Get(ical.PropDateTimeEnd); endProp != nil {
		if end, err := endProp.DateTime(time.UTC); err == nil {
			if d := end.Sub(start); d > 0 {
				return d
			}
		}
	}
	if durProp := ev.Props.Get(ical.PropDuration); durProp != nil {
		if d, err := durProp.Duration(); err == nil && d > 0 {
			return d
		}
	}
	return 0
}

// eventOverlaps reports whether a single VEVENT (expanding any RRULE) overlaps
// the range.
func eventOverlaps(ev *ical.Component, tr timeRange) bool {
	start, err := ev.Props.DateTime(ical.PropDateTimeStart, time.UTC)
	if err != nil {
		return false
	}
	dur := eventDuration(ev)

	set, err := ev.RecurrenceSet(time.UTC)
	if err == nil && set != nil {
		return recurrenceOverlaps(set, dur, tr)
	}

	// Non-recurring (or unparseable recurrence): test the single instance.
	end := start.Add(dur)
	return tr.overlaps(start, end)
}

// recurrenceOverlaps expands a recurrence set within the query window and
// reports whether any instance overlaps tr. Each instance is treated as
// having the master event's duration.
func recurrenceOverlaps(set recurrenceSet, dur time.Duration, tr timeRange) bool {
	// Build a query window for the set. Since an instance starting before
	// the range can still overlap it (via its duration), shift the lower
	// bound back by the event duration. Use generous sentinels when the
	// range is unbounded.
	after := tr.Start
	if after.IsZero() {
		after = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	} else {
		after = after.Add(-dur)
	}
	before := tr.End
	if before.IsZero() {
		before = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}

	for _, occ := range set.Between(after, before, true) {
		if tr.overlaps(occ, occ.Add(dur)) {
			return true
		}
	}
	return false
}

// recurrenceSet is the subset of *rrule.Set used here; defining it as an
// interface keeps recurrenceOverlaps independently testable.
type recurrenceSet interface {
	Between(after, before time.Time, inc bool) []time.Time
}
