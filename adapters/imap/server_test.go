package imap_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	imapadapter "github.com/vul-os/vmail/adapters/imap"
	"github.com/vul-os/vmail/internal/account"
	"github.com/vul-os/vmail/internal/blob"
	"github.com/vul-os/vmail/internal/eventlog"
	"github.com/vul-os/vmail/internal/ids"
	"github.com/vul-os/vmail/internal/model"
)

func mkmsg(id, subject, body string) []byte {
	return []byte(fmt.Sprintf("From: alice@example.com\r\nTo: bob@vmail.test\r\nSubject: %s\r\nMessage-ID: <%s>\r\nDate: Mon, 02 Jan 2006 15:04:05 -0700\r\n\r\n%s\r\n", subject, id, body))
}

// Full client↔server round trip against the real emersion/go-imap client.
func TestIMAPClientRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, _ := blob.NewFS(t.TempDir())
	log := eventlog.NewMem(func() time.Time { return time.Unix(0, 0).UTC() })
	rt, err := account.Open(ctx, log, store, ids.NewGen(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Ingest(ctx, mkmsg("a@x", "Hello Alpha", "first body zebra"), []model.LabelID{model.LabelInbox}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Ingest(ctx, mkmsg("b@x", "Hello Beta", "second body"), []model.LabelID{model.LabelInbox}, nil); err != nil {
		t.Fatal(err)
	}

	be := &imapadapter.Backend{Auth: func(string, string) (*account.Runtime, error) { return rt, nil }}
	srv := imapadapter.NewServer(be, nil)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go srv.Serve(ln)

	c, err := imapclient.DialInsecure(ln.Addr().String(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Login("bob", "pw").Wait(); err != nil {
		t.Fatalf("login: %v", err)
	}

	sel, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if sel.NumMessages != 2 {
		t.Fatalf("select NumMessages = %d, want 2", sel.NumMessages)
	}

	// FETCH envelope + flags + full body for both messages.
	bufs, err := c.Fetch(imap.SeqSetNum(1, 2), &imap.FetchOptions{
		Envelope:    true,
		Flags:       true,
		UID:         true,
		BodySection: []*imap.FetchItemBodySection{{Peek: true}}, // PEEK: don't mark \Seen
	}).Collect()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(bufs) != 2 {
		t.Fatalf("fetched %d messages, want 2", len(bufs))
	}
	if bufs[0].Envelope == nil || !strings.Contains(bufs[0].Envelope.Subject, "Alpha") {
		t.Errorf("envelope subject wrong: %+v", bufs[0].Envelope)
	}
	if len(bufs[0].BodySection) == 0 || !strings.Contains(string(bufs[0].BodySection[0].Bytes), "zebra") {
		t.Errorf("body section missing 'zebra'")
	}

	// STORE \Seen on message 1.
	if _, err := c.Store(imap.SeqSetNum(1), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagSeen}}, nil).Collect(); err != nil {
		t.Fatalf("store: %v", err)
	}

	// SEARCH unseen -> message 2 only. (Under IMAP4rev1 the result is the number
	// list in All; Count is an ESEARCH/rev2 field and stays 0 here.)
	sd, err := c.Search(&imap.SearchCriteria{NotFlag: []imap.Flag{imap.FlagSeen}}, nil).Wait()
	if err != nil {
		t.Fatalf("search unseen: %v", err)
	}
	if got := allString(sd.All); got != "2" {
		t.Errorf("unseen search = %q, want \"2\"", got)
	}

	// SEARCH body 'zebra' -> message 1 only.
	sd2, err := c.Search(&imap.SearchCriteria{Body: []string{"zebra"}}, nil).Wait()
	if err != nil {
		t.Fatalf("search body: %v", err)
	}
	if got := allString(sd2.All); got != "1" {
		t.Errorf("body search = %q, want \"1\"", got)
	}

	// APPEND a new message, then confirm it appears.
	appended := mkmsg("c@x", "Appended", "via append path")
	ac := c.Append("INBOX", int64(len(appended)), nil)
	if _, err := ac.Write(appended); err != nil {
		t.Fatalf("append write: %v", err)
	}
	if err := ac.Close(); err != nil {
		t.Fatalf("append close: %v", err)
	}
	if _, err := ac.Wait(); err != nil {
		t.Fatalf("append: %v", err)
	}
	sel2, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if sel2.NumMessages != 3 {
		t.Errorf("after append NumMessages = %d, want 3", sel2.NumMessages)
	}

	// Mark message 1 \Deleted and EXPUNGE; inbox drops to 2.
	if _, err := c.Store(imap.SeqSetNum(1), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Collect(); err != nil {
		t.Fatalf("store deleted: %v", err)
	}
	if _, err := c.Expunge().Collect(); err != nil {
		t.Fatalf("expunge: %v", err)
	}
	sel3, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		t.Fatal(err)
	}
	if sel3.NumMessages != 2 {
		t.Errorf("after expunge NumMessages = %d, want 2", sel3.NumMessages)
	}

	_ = c.Logout().Wait()
}

// allString renders a SEARCH result set as a comma-free range string ("1", "2",
// "1:3"...), independent of ESEARCH availability.
func allString(ns imap.NumSet) string {
	switch s := ns.(type) {
	case imap.SeqSet:
		return s.String()
	case imap.UIDSet:
		return s.String()
	default:
		return fmt.Sprint(ns)
	}
}
