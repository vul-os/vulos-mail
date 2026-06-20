// Package mime parses RFC 5322 / MIME messages using emersion/go-message. It
// replaces the stdlib net/mail parsing used in the Wave-3 slice: go-message
// handles encoded-word headers, multipart bodies, and gives us inline text
// extraction for search/embeddings.
package mime

import (
	"bytes"
	"io"
	"strings"

	gomail "github.com/emersion/go-message/mail"

	"github.com/vul-os/vmail/internal/model"
)

// ParseEnvelope extracts the protocol-free envelope from a raw message.
func ParseEnvelope(raw []byte) (model.Envelope, error) {
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return model.Envelope{}, err
	}
	h := mr.Header
	env := model.Envelope{
		From: addrList(h, "From"),
		To:   addrList(h, "To"),
		Cc:   addrList(h, "Cc"),
	}
	env.Subject, _ = h.Subject()
	if d, err := h.Date(); err == nil {
		env.Date = d.UTC()
	}
	if fal, err := h.AddressList("From"); err == nil && len(fal) > 0 {
		env.FromName = fal[0].Name
	}
	if mid, err := h.MessageID(); err == nil {
		env.MessageIDHeader = mid
	}
	if irt, err := h.MsgIDList("In-Reply-To"); err == nil && len(irt) > 0 {
		env.InReplyTo = irt[0]
	}
	if refs, err := h.MsgIDList("References"); err == nil {
		env.References = refs
	}
	return env, nil
}

// ExtractText returns the concatenated inline (text/plain, text/html) body text,
// for use as the search/FTS source. Attachments are skipped. HTML is returned
// as-is for now (tag-stripping is a later refinement).
func ExtractText(raw []byte) (string, error) {
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return sb.String(), err
		}
		if _, ok := p.Header.(*gomail.InlineHeader); ok {
			b, _ := io.ReadAll(p.Body)
			sb.Write(b)
			sb.WriteByte('\n')
		}
	}
	return sb.String(), nil
}

// ExtractAttachments returns the decoded bodies of attachment parts (for hash
// scanning, e.g. CSAM detection).
func ExtractAttachments(raw []byte) [][]byte {
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	var out [][]byte
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if _, ok := p.Header.(*gomail.AttachmentHeader); ok {
			if b, err := io.ReadAll(p.Body); err == nil {
				out = append(out, b)
			}
		}
	}
	return out
}

// Attachment is a decoded attachment part with its metadata.
type Attachment struct {
	Name string
	Type string
	Data []byte
}

// Attachments returns all attachment parts (name, type, bytes) in order.
func Attachments(raw []byte) []Attachment {
	mr, err := gomail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	var out []Attachment
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		ah, ok := p.Header.(*gomail.AttachmentHeader)
		if !ok {
			continue
		}
		name, _ := ah.Filename()
		ct, _, _ := ah.ContentType()
		data, _ := io.ReadAll(p.Body)
		out = append(out, Attachment{Name: name, Type: ct, Data: data})
	}
	return out
}

// AttachmentAt returns the i-th attachment (0-based), or false if out of range.
func AttachmentAt(raw []byte, i int) (Attachment, bool) {
	atts := Attachments(raw)
	if i < 0 || i >= len(atts) {
		return Attachment{}, false
	}
	return atts[i], true
}

func addrList(h gomail.Header, key string) []string {
	al, err := h.AddressList(key)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(al))
	for _, a := range al {
		if a != nil {
			out = append(out, a.Address)
		}
	}
	return out
}
