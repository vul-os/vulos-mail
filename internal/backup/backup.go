// Package backup exports an account's mail to standard, portable formats
// (mbox and Maildir), reconstructed from the event log and blob store.
//
// Nothing here mutates state: it rebuilds the account projection from the log,
// enumerates messages in ingest order, fetches each raw RFC822 body from the
// blob store, and renders it into the requested container format. The result is
// a clean, tool-readable archive that any mail client can import.
package backup

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/model"
	"github.com/vul-os/vmail/internal/projection"
)

// ExportMbox writes every (non-trash, non-spam) message to w in mbox format.
//
// Each message is preceded by a "From <addr> <timestamp>\r\n" separator line
// derived from the envelope From and Date, and body lines beginning with "From "
// are escaped with the standard mbox ">From " quoting so the archive round-trips.
func ExportMbox(ctx context.Context, log eventlog.Log, store blob.Store, w io.Writer) error {
	acc, err := projection.Rebuild(ctx, log)
	if err != nil {
		return fmt.Errorf("backup: rebuild account: %w", err)
	}
	bw := bufio.NewWriter(w)
	for _, m := range acc.AllMail() {
		body, err := store.Get(ctx, m.BlobRef)
		if err != nil {
			return fmt.Errorf("backup: get blob %s: %w", m.BlobRef, err)
		}
		sep := mboxSeparator(m)
		if _, err := bw.WriteString(sep); err != nil {
			return err
		}
		if err := writeMboxBody(bw, body); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// mboxSeparator builds the "From " line that precedes a message in an mbox.
func mboxSeparator(m *model.Message) string {
	addr := "MAILER-DAEMON"
	if len(m.Envelope.From) > 0 && m.Envelope.From[0] != "" {
		addr = m.Envelope.From[0]
	}
	t := m.Envelope.Date
	if t.IsZero() {
		t = time.Unix(0, 0).UTC()
	}
	// asctime form, as used by the traditional Unix mbox "From " line.
	return fmt.Sprintf("From %s %s\r\n", addr, t.UTC().Format("Mon Jan _2 15:04:05 2006"))
}

// writeMboxBody writes a raw RFC822 body with mbox ">From " line quoting and a
// trailing blank line separating it from the next message.
func writeMboxBody(w *bufio.Writer, body []byte) error {
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		trimmed := bytes.TrimRight(line, "\r")
		if isFromLine(trimmed) {
			if _, err := w.WriteString(">"); err != nil {
				return err
			}
		}
		if _, err := w.Write(trimmed); err != nil {
			return err
		}
		if _, err := w.WriteString("\r\n"); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	// Blank line terminates the message in the mbox stream.
	_, err := w.WriteString("\r\n")
	return err
}

// isFromLine reports whether a line begins with "From " (after stripping any
// leading ">" quoting), i.e. it needs mbox escaping.
func isFromLine(line []byte) bool {
	for len(line) > 0 && line[0] == '>' {
		line = line[1:]
	}
	return bytes.HasPrefix(line, []byte("From "))
}

// ExportMaildir writes every (non-trash, non-spam) message into a Maildir at
// dir, creating the standard tmp/, new/, and cur/ subdirectories. Each message
// is written to cur/ under a unique name carrying Maildir info flags derived
// from the message flags (e.g. ":2,S" for Seen, ":2,RS" for Replied+Seen).
func ExportMaildir(ctx context.Context, log eventlog.Log, store blob.Store, dir string) error {
	acc, err := projection.Rebuild(ctx, log)
	if err != nil {
		return fmt.Errorf("backup: rebuild account: %w", err)
	}
	for _, sub := range []string{"tmp", "new", "cur"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o700); err != nil {
			return fmt.Errorf("backup: mkdir maildir: %w", err)
		}
	}
	cur := filepath.Join(dir, "cur")
	for i, m := range acc.AllMail() {
		body, err := store.Get(ctx, m.BlobRef)
		if err != nil {
			return fmt.Errorf("backup: get blob %s: %w", m.BlobRef, err)
		}
		name := maildirName(m, i) + maildirFlags(m.Flags)
		p := filepath.Join(cur, name)
		if err := os.WriteFile(p, body, 0o600); err != nil {
			return fmt.Errorf("backup: write maildir file: %w", err)
		}
	}
	return nil
}

// maildirName builds the unique-part of a Maildir filename. It is derived from
// stable inputs (a counter plus a hash of the message id/blob ref) so an export
// is deterministic and collision-free without depending on wall-clock or PID.
func maildirName(m *model.Message, i int) string {
	h := sha256.Sum256([]byte(string(m.ID) + "|" + string(m.BlobRef)))
	return strconv.Itoa(i) + "." + hex.EncodeToString(h[:8]) + ".vmail"
}

// maildirFlags renders the Maildir info suffix (":2,<FLAGS>") for a message's
// flag set. Flag letters are emitted in their canonical ASCII-sorted order.
func maildirFlags(flags map[model.Flag]bool) string {
	var letters []string
	if flags[model.FlagDraft] {
		letters = append(letters, "D")
	}
	if flags[model.FlagFlagged] {
		letters = append(letters, "F")
	}
	if flags[model.FlagAnswered] {
		letters = append(letters, "R")
	}
	if flags[model.FlagSeen] {
		letters = append(letters, "S")
	}
	sort.Strings(letters)
	return ":2," + strings.Join(letters, "")
}
