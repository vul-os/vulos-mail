package smtpin

import "testing"

// stripAuthResults runs on every inbound message; it must never panic and must
// be idempotent (a second pass changes nothing).
func FuzzStripAuthResults(f *testing.F) {
	f.Add([]byte("Authentication-Results: mx; dmarc=pass\r\nFrom: a@b\r\n\r\nbody"), "mx")
	f.Add([]byte("From: a@b\r\n\r\nx"), "mx.vulos.to")
	f.Add([]byte(""), "")
	f.Fuzz(func(t *testing.T, raw []byte, sid string) {
		out := stripAuthResults(raw, sid)
		again := stripAuthResults(out, sid)
		if string(out) != string(again) {
			t.Fatal("stripAuthResults is not idempotent")
		}
	})
}
