package compose

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// headerLines returns the raw header field lines (before the blank line that
// separates headers from body), unfolded onto whether each starts a new field.
func headerLines(t *testing.T, raw []byte) []string {
	t.Helper()
	idx := bytes.Index(raw, []byte("\r\n\r\n"))
	if idx < 0 {
		idx = len(raw)
	}
	var lines []string
	sc := bufio.NewScanner(bytes.NewReader(raw[:idx]))
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}

// fieldNames returns the set of header field names that begin a line (a folded
// continuation line starts with whitespace and is NOT a new field). A CRLF
// injection would manifest as an attacker-chosen field name appearing here.
func fieldNames(lines []string) map[string]bool {
	out := map[string]bool{}
	for _, l := range lines {
		if l == "" || l[0] == ' ' || l[0] == '\t' {
			continue // folded continuation
		}
		if i := strings.IndexByte(l, ':'); i > 0 {
			out[strings.ToLower(strings.TrimSpace(l[:i]))] = true
		}
	}
	return out
}

// TestNoHeaderInjection verifies that CRLF sequences embedded in attacker-
// controllable fields (Subject, To/Cc/From addresses, including unparseable
// ones) can never introduce a NEW header line such as Bcc:. go-message must
// either encoded-word the value (Subject) or fold it into a quoted local-part
// (addresses); in neither case may a fresh "bcc:" field appear.
func TestNoHeaderInjection(t *testing.T) {
	const evil = "evil@attacker.example"
	cases := map[string]Message{
		"subject":       {From: "a@x.com", To: []string{"b@y.com"}, Subject: "Hi\r\nBcc: " + evil, Text: "x"},
		"to_address":    {From: "a@x.com", To: []string{"b@y.com\r\nBcc: " + evil}, Subject: "s", Text: "x"},
		"to_unparsable": {From: "a@x.com", To: []string{"\"name\r\nBcc: " + evil + "\" <b@y.com>"}, Subject: "s", Text: "x"},
		"cc_address":    {From: "a@x.com", To: []string{"b@y.com"}, Cc: []string{"c@y.com\r\nBcc: " + evil}, Subject: "s", Text: "x"},
		"from_address":  {From: "a@x.com\r\nBcc: " + evil, To: []string{"b@y.com"}, Subject: "s", Text: "x"},
		"subject_lf":    {From: "a@x.com", To: []string{"b@y.com"}, Subject: "Hi\nBcc: " + evil, Text: "x"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			raw, err := Build(m)
			if err != nil {
				return // rejecting the message outright is also safe
			}
			names := fieldNames(headerLines(t, raw))
			if names["bcc"] {
				t.Fatalf("CRLF header injection: a Bcc field was introduced\n%s", raw)
			}
			// The raw header block must not contain a bare line that is exactly an
			// injected Bcc header value either.
			for _, l := range headerLines(t, raw) {
				if strings.HasPrefix(strings.ToLower(l), "bcc:") {
					t.Fatalf("CRLF header injection: stray Bcc line %q", l)
				}
			}
		})
	}
}
