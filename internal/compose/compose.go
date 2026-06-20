// Package compose builds RFC 5322 / MIME messages (text, HTML, attachments)
// using emersion/go-message. Shared by the webmail compose path and the
// transactional API so message construction lives in one place.
package compose

import (
	"bytes"
	"net/mail"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

// Attachment is a file part.
type Attachment struct {
	Name string
	Type string // MIME type; defaults to application/octet-stream
	Data []byte
}

// Message describes a message to build.
type Message struct {
	From        string
	To          []string
	Cc          []string
	Subject     string
	Text        string
	HTML        string
	Date        time.Time
	MessageID   string // bare id (no angle brackets)
	Attachments []Attachment
}

// Build serializes m to RFC822 bytes. The body is multipart/alternative when both
// text and HTML are present; attachments wrap it in multipart/mixed.
func Build(m Message) ([]byte, error) {
	var buf bytes.Buffer
	var h gomail.Header
	if m.Date.IsZero() {
		m.Date = time.Now()
	}
	h.SetDate(m.Date.UTC())
	h.SetSubject(m.Subject)
	if m.MessageID != "" {
		h.SetMessageID(m.MessageID)
	}
	h.SetAddressList("From", addrs([]string{m.From}))
	h.SetAddressList("To", addrs(m.To))
	if len(m.Cc) > 0 {
		h.SetAddressList("Cc", addrs(m.Cc))
	}

	mw, err := gomail.CreateWriter(&buf, h)
	if err != nil {
		return nil, err
	}

	iw, err := mw.CreateInline()
	if err != nil {
		return nil, err
	}
	if m.Text != "" || m.HTML == "" {
		if err := inlinePart(iw, "text/plain", m.Text); err != nil {
			return nil, err
		}
	}
	if m.HTML != "" {
		if err := inlinePart(iw, "text/html", m.HTML); err != nil {
			return nil, err
		}
	}
	if err := iw.Close(); err != nil {
		return nil, err
	}

	for _, a := range m.Attachments {
		var ah gomail.AttachmentHeader
		ct := a.Type
		if ct == "" {
			ct = "application/octet-stream"
		}
		ah.SetContentType(ct, nil)
		ah.SetFilename(a.Name)
		w, err := mw.CreateAttachment(ah)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(a.Data); err != nil {
			w.Close()
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func inlinePart(iw *gomail.InlineWriter, contentType, body string) error {
	var ih gomail.InlineHeader
	ih.SetContentType(contentType, map[string]string{"charset": "utf-8"})
	w, err := iw.CreatePart(ih)
	if err != nil {
		return err
	}
	if _, err := w.Write([]byte(body)); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}

func addrs(ss []string) []*gomail.Address {
	out := make([]*gomail.Address, 0, len(ss))
	for _, s := range ss {
		if a, err := mail.ParseAddress(s); err == nil {
			out = append(out, &gomail.Address{Name: a.Name, Address: a.Address})
		} else if s != "" {
			out = append(out, &gomail.Address{Address: s})
		}
	}
	return out
}
