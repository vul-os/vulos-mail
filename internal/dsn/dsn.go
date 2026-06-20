// Package dsn builds Delivery Status Notifications (bounce messages) returned to
// a sender when outbound delivery permanently fails.
package dsn

import (
	"fmt"
	"strings"
)

// Build returns a bounce message from MAILER-DAEMON@reportingDomain to the
// original sender, listing the failed recipients and the reason. (A full
// multipart/report per RFC 3464 is a later refinement; this is a clear,
// client-readable text bounce.)
func Build(reportingDomain, sender string, recipients []string, reason string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: MAILER-DAEMON@%s\r\n", reportingDomain)
	fmt.Fprintf(&b, "To: %s\r\n", sender)
	b.WriteString("Subject: Undelivered Mail Returned to Sender\r\n")
	b.WriteString("Auto-Submitted: auto-replied\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString("Your message could not be delivered to one or more recipients:\r\n\r\n")
	for _, r := range recipients {
		fmt.Fprintf(&b, "    %s\r\n", r)
	}
	fmt.Fprintf(&b, "\r\nReason: %s\r\n", reason)
	return []byte(b.String())
}
