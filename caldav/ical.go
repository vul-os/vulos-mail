package caldav

import (
	"bytes"

	"github.com/emersion/go-ical"
)

// validICS reports whether b decodes as a syntactically valid iCalendar
// object. It is used to reject malformed PUT bodies before storage.
func validICS(b []byte) bool {
	_, err := ical.NewDecoder(bytes.NewReader(b)).Decode()
	return err == nil
}
