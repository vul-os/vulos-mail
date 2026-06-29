package mtaout

// Internal tests (package mtaout, not mtaout_test) so we can call the
// unexported deliver method directly over an in-memory net.Pipe connection,
// giving deterministic coverage without real DNS or network.

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// runFakeSMTPServer drives the server side of a net.Pipe connection, acting as
// a minimal SMTP MX. If offerSTARTTLS is true, it includes STARTTLS in the
// EHLO response. It records every command the client sends and returns them
// (in upper-case) on doneCh when the session ends.
func runFakeSMTPServer(conn net.Conn, offerSTARTTLS bool, doneCh chan<- []string) {
	go func() {
		defer conn.Close()
		var cmds []string

		fmt.Fprintf(conn, "220 test.mx ESMTP\r\n")

		scanner := bufio.NewScanner(conn)
		for scanner.Scan() {
			line := scanner.Text()
			upper := strings.ToUpper(strings.TrimSpace(line))
			cmds = append(cmds, upper)

			switch {
			case strings.HasPrefix(upper, "EHLO"):
				if offerSTARTTLS {
					fmt.Fprintf(conn, "250-test.mx\r\n250 STARTTLS\r\n")
				} else {
					fmt.Fprintf(conn, "250 test.mx\r\n")
				}
			case upper == "STARTTLS":
				// Signal that the STARTTLS command was received, then end — the TLS
				// handshake will fail (no server cert on a pipe) which is expected.
				fmt.Fprintf(conn, "220 Ready to start TLS\r\n")
				doneCh <- cmds
				return
			case strings.HasPrefix(upper, "MAIL FROM"):
				fmt.Fprintf(conn, "250 ok\r\n")
			case strings.HasPrefix(upper, "RCPT TO"):
				fmt.Fprintf(conn, "250 ok\r\n")
			case upper == "DATA":
				fmt.Fprintf(conn, "354 start\r\n")
			case upper == "QUIT":
				fmt.Fprintf(conn, "221 bye\r\n")
				doneCh <- cmds
				return
			default:
				fmt.Fprintf(conn, "250 ok\r\n")
			}
		}
		// Scanner ended (EOF / closed conn) — deliver returned without QUIT.
		doneCh <- cmds
	}()
}

func TestSTARTTLSAttemptedWhenOffered(t *testing.T) {
	client, server := net.Pipe()
	doneCh := make(chan []string, 1)
	runFakeSMTPServer(server, true /* offerSTARTTLS */, doneCh)

	s := &SMTPSender{HELO: "mail.example.com"}
	msg := OutMessage{
		From:  "alice@example.com",
		Rcpts: []string{"bob@remote.example"},
		Raw:   []byte("From: alice@example.com\r\nTo: bob@remote.example\r\nSubject: t\r\n\r\nhi\r\n"),
	}
	// deliver will fail at the TLS handshake (no server cert) — that is expected.
	// We only care that the STARTTLS command was issued before the failure.
	_ = s.deliver("test.mx", client, msg)

	select {
	case cmds := <-doneCh:
		found := false
		for _, c := range cmds {
			if c == "STARTTLS" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("STARTTLS command not sent to server; commands seen: %v", cmds)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for fake SMTP server to complete")
	}
}

func TestSTARTTLSEnforceFailsWhenNotOffered(t *testing.T) {
	client, server := net.Pipe()
	doneCh := make(chan []string, 1)
	runFakeSMTPServer(server, false /* no STARTTLS */, doneCh)

	s := &SMTPSender{HELO: "mail.example.com", STARTTLSEnforce: true}
	msg := OutMessage{
		From:  "alice@example.com",
		Rcpts: []string{"bob@remote.example"},
		Raw:   []byte("From: alice@example.com\r\nTo: bob@remote.example\r\nSubject: t\r\n\r\nhi\r\n"),
	}
	res := s.deliver("test.mx", client, msg)

	if res.Status == Delivered {
		t.Error("enforce mode must not deliver when STARTTLS is not offered by the remote")
	}
	if res.Status != TempFail {
		t.Errorf("expected TempFail (retriable), got %v", res.Status)
	}
	if res.Err == nil || !strings.Contains(res.Err.Error(), "STARTTLS required") {
		t.Errorf("error should mention STARTTLS requirement; got: %v", res.Err)
	}

	// Drain server goroutine.
	select {
	case <-doneCh:
	case <-time.After(3 * time.Second):
		t.Error("timeout draining fake SMTP server")
	}
}
