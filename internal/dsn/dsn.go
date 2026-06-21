// Package dsn builds Delivery Status Notifications (bounce messages) returned to
// a sender when outbound delivery permanently fails. The format is a
// multipart/report (RFC 3464): a human-readable part plus a machine-readable
// message/delivery-status part that mail clients and MTAs can parse.
package dsn

import (
	"fmt"
	"strings"
)

// Build returns a bounce message from MAILER-DAEMON@reportingDomain to the
// original sender, listing the failed recipients and the reason, as a
// multipart/report (report-type=delivery-status) per RFC 3464.
func Build(reportingDomain, sender string, recipients []string, reason string) []byte {
	const boundary = "vulos-dsn-boundary-9d2f"
	reason = sanitizeHeader(reason)

	var b strings.Builder
	fmt.Fprintf(&b, "From: MAILER-DAEMON@%s\r\n", reportingDomain)
	fmt.Fprintf(&b, "To: %s\r\n", sender)
	b.WriteString("Subject: Undelivered Mail Returned to Sender\r\n")
	b.WriteString("Auto-Submitted: auto-replied\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/report; report-type=delivery-status; boundary=\"%s\"\r\n", boundary)
	b.WriteString("\r\n")
	b.WriteString("This is a MIME-encapsulated delivery status notification.\r\n\r\n")

	// Part 1 — human-readable explanation.
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString("Your message could not be delivered to one or more recipients:\r\n\r\n")
	for _, r := range recipients {
		fmt.Fprintf(&b, "    %s\r\n", r)
	}
	fmt.Fprintf(&b, "\r\nReason: %s\r\n\r\n", reason)

	// Part 2 — machine-readable delivery-status (RFC 3464 §2.1).
	fmt.Fprintf(&b, "--%s\r\n", boundary)
	b.WriteString("Content-Type: message/delivery-status\r\n\r\n")
	fmt.Fprintf(&b, "Reporting-MTA: dns; %s\r\n", reportingDomain)
	for _, r := range recipients {
		b.WriteString("\r\n")
		fmt.Fprintf(&b, "Final-Recipient: rfc822; %s\r\n", sanitizeHeader(r))
		b.WriteString("Action: failed\r\n")
		b.WriteString("Status: 5.0.0\r\n")
		fmt.Fprintf(&b, "Diagnostic-Code: smtp; %s\r\n", reason)
	}
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return []byte(b.String())
}

// sanitizeHeader strips CR/LF so a reason/recipient can't inject headers or
// extra MIME parts into the generated DSN.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}
