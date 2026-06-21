package event_test

import (
	"testing"

	"github.com/vul-os/vulos-mail/internal/event"
)

// Decode is the durable log codec applied to bytes read from disk; corrupt or
// hostile input must error, never panic.
func FuzzDecode(f *testing.F) {
	for _, e := range []event.Event{
		event.Labeled{MessageID: "m", LabelID: "inbox"},
		event.FlagSet{MessageID: "m", Flag: "\\Seen", Value: true},
	} {
		if b, err := event.Encode(e); err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte("{}"))
	f.Add([]byte(`{"kind":"x","data":null}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = event.Decode(data)
	})
}
