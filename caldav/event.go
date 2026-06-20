package caldav

import (
	"bytes"
	"time"

	"github.com/emersion/go-ical"
)

// Event is a minimal calendar event projection (the webmail's model).
type Event struct {
	UID     string    `json:"id"`
	Summary string    `json:"summary"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
}

// BuildEvent serializes an event to a VCALENDAR/VEVENT body.
func BuildEvent(e Event) ([]byte, error) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, "-//vulos-mail//webmail//EN")
	cal.Props.SetText(ical.PropVersion, "2.0")

	ev := ical.NewEvent()
	ev.Props.SetText(ical.PropUID, e.UID)
	ev.Props.SetDateTime(ical.PropDateTimeStamp, time.Now().UTC())
	ev.Props.SetText(ical.PropSummary, e.Summary)
	ev.Props.SetDateTime(ical.PropDateTimeStart, e.Start.UTC())
	if !e.End.IsZero() {
		ev.Props.SetDateTime(ical.PropDateTimeEnd, e.End.UTC())
	}
	cal.Children = append(cal.Children, ev.Component)

	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ParseEvents extracts events from a VCALENDAR body.
func ParseEvents(ics []byte) []Event {
	cal, err := ical.NewDecoder(bytes.NewReader(ics)).Decode()
	if err != nil {
		return nil
	}
	var out []Event
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		ev := &ical.Event{Component: child}
		uid, _ := ev.Props.Text(ical.PropUID)
		summary, _ := ev.Props.Text(ical.PropSummary)
		start, _ := ev.DateTimeStart(time.UTC)
		end, _ := ev.DateTimeEnd(time.UTC)
		out = append(out, Event{UID: uid, Summary: summary, Start: start, End: end})
	}
	return out
}
