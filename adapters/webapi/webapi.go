// Package webapi provides a SendGrid/Mailgun-style transactional email
// HTTP/JSON API exposed as an [http.Handler].
//
// The package is deliberately decoupled from the rest of the mail system: it
// depends only on the standard library and github.com/emersion/go-message for
// composing MIME. Integration points (API-key authentication and message
// submission for delivery) are injected as functions on [Backend], so callers
// wire it to their own auth store and mail pipeline without this package
// importing any internal package.
package webapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

// Backend holds the injected dependencies the API needs to authenticate
// requests and hand composed messages off for delivery. Both fields must be
// set before calling [Backend.Handler].
type Backend struct {
	// AuthKey validates a Bearer API key. It returns the owning account ID and
	// true if the key is valid, or ("", false) otherwise.
	AuthKey func(apiKey string) (accountID string, ok bool)

	// Submit hands a composed RFC 822 message off for delivery. from is the
	// envelope sender, to is the full list of envelope recipients (To and Cc),
	// and raw is the serialized MIME message. A non-nil error indicates the
	// message could not be accepted for delivery.
	Submit func(ctx context.Context, from string, to []string, raw []byte) error
}

// Handler returns an [http.Handler] exposing the transactional send API.
//
// Routes:
//
//	POST /api/v1/send  — compose and submit a message (Bearer auth required)
func (b *Backend) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/send", b.handleSend)
	return mux
}

// sendRequest is the JSON body accepted by POST /api/v1/send.
type sendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Cc      []string `json:"cc"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

// sendResponse is the JSON body returned on a successful send.
type sendResponse struct {
	ID       string `json:"id"`
	Accepted int    `json:"accepted"`
}

// errorResponse is the JSON body returned for error status codes.
type errorResponse struct {
	Error string `json:"error"`
}

func (b *Backend) handleSend(w http.ResponseWriter, r *http.Request) {
	key, ok := bearerToken(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
		return
	}
	if _, ok := b.AuthKey(key); !ok {
		writeError(w, http.StatusUnauthorized, "invalid API key")
		return
	}

	var req sendRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "malformed JSON: "+err.Error())
		return
	}

	if strings.TrimSpace(req.From) == "" {
		writeError(w, http.StatusBadRequest, "missing required field: from")
		return
	}
	if len(req.To) == 0 {
		writeError(w, http.StatusBadRequest, "missing required field: to")
		return
	}
	if strings.TrimSpace(req.Subject) == "" {
		writeError(w, http.StatusBadRequest, "missing required field: subject")
		return
	}

	recipients := make([]string, 0, len(req.To)+len(req.Cc))
	recipients = append(recipients, req.To...)
	recipients = append(recipients, req.Cc...)

	id := messageID(domainOf(req.From))
	raw, err := compose(id, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not compose message: "+err.Error())
		return
	}

	if err := b.Submit(r.Context(), req.From, recipients, raw); err != nil {
		writeError(w, http.StatusBadGateway, "delivery failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, sendResponse{ID: id, Accepted: len(recipients)})
}

// compose builds an RFC 822 MIME message for req using the given Message-ID
// (without angle brackets). When both Text and HTML are supplied the message is
// a multipart/alternative; when only one is supplied it is a single-part
// message of the corresponding type.
func compose(id string, req sendRequest) ([]byte, error) {
	from, err := parseAddressList(req.From)
	if err != nil {
		return nil, fmt.Errorf("from: %w", err)
	}
	to, err := parseAddressList(strings.Join(req.To, ", "))
	if err != nil {
		return nil, fmt.Errorf("to: %w", err)
	}

	var h gomail.Header
	h.SetAddressList("From", from)
	h.SetAddressList("To", to)
	if len(req.Cc) > 0 {
		cc, err := parseAddressList(strings.Join(req.Cc, ", "))
		if err != nil {
			return nil, fmt.Errorf("cc: %w", err)
		}
		h.SetAddressList("Cc", cc)
	}
	h.SetDate(time.Now())
	h.SetSubject(req.Subject)
	h.SetMessageID(id)

	hasText := req.Text != ""
	hasHTML := req.HTML != ""
	if !hasText && !hasHTML {
		return nil, errors.New("at least one of text or html must be provided")
	}

	var buf bytes.Buffer
	mw, err := gomail.CreateWriter(&buf, h)
	if err != nil {
		return nil, err
	}

	iw, err := mw.CreateInline()
	if err != nil {
		return nil, err
	}
	if hasText {
		if err := writeInlinePart(iw, "text/plain", req.Text); err != nil {
			return nil, err
		}
	}
	if hasHTML {
		if err := writeInlinePart(iw, "text/html", req.HTML); err != nil {
			return nil, err
		}
	}
	if err := iw.Close(); err != nil {
		return nil, err
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// writeInlinePart writes a single inline text part with the given MIME type
// (e.g. "text/plain" or "text/html") and UTF-8 body.
func writeInlinePart(iw *gomail.InlineWriter, contentType, body string) error {
	var ih gomail.InlineHeader
	ih.Set("Content-Type", contentType+"; charset=utf-8")
	pw, err := iw.CreatePart(ih)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(pw, body); err != nil {
		return err
	}
	return pw.Close()
}

// parseAddressList parses a comma-separated RFC 5322 address list into the form
// expected by go-message/mail.
func parseAddressList(s string) ([]*gomail.Address, error) {
	parsed, err := mail.ParseAddressList(s)
	if err != nil {
		return nil, err
	}
	out := make([]*gomail.Address, len(parsed))
	for i, a := range parsed {
		out[i] = &gomail.Address{Name: a.Name, Address: a.Address}
	}
	return out, nil
}

// messageID generates a unique Message-ID (without surrounding angle brackets)
// of the form "<random>@<domain>". If domain is empty, "localhost" is used.
func messageID(domain string) string {
	if domain == "" {
		domain = "localhost"
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand failure is unexpected; fall back to a time-based value so
		// callers still receive a usable, reasonably unique ID.
		return fmt.Sprintf("%d@%s", time.Now().UnixNano(), domain)
	}
	return hex.EncodeToString(b[:]) + "@" + domain
}

// domainOf extracts the domain portion of an address such as "a@b.com". It
// accepts either a bare address or a full "Name <addr>" form and returns an
// empty string if no domain can be determined.
func domainOf(addr string) string {
	if a, err := mail.ParseAddress(addr); err == nil {
		addr = a.Address
	}
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return ""
}

// bearerToken extracts the token from an "Authorization: Bearer <token>"
// header. It returns ("", false) if the header is missing or malformed.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(h[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
