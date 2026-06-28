package main

// import_handlers.go — Broker-gated bulk PIM import endpoints.
//
// POST /api/admin/import/contacts — bulk vCards into the CardDAV contacts store.
// POST /api/admin/import/events   — bulk iCalendar events into the CalDAV store.
//
// These endpoints are called by the Vulos OS import engine (backend/services/files)
// when a user runs a contacts or calendar import job. The OS pulls data from the
// connected provider (Google Contacts/Calendar, Microsoft Graph), maps it to
// vCard / iCal format, then POSTs it here in bulk so vulos-mail stores
// the user's OWNED COPIES that persist even after the integration is disconnected.
//
// Auth: X-Vulos-Broker-Auth / LILMAIL_BROKER_SECRET (same pattern as the other
// admin/brokered routes: provision-mailbox, diagnostics).
//
// Idempotency: contacts are keyed by their vCard UID field; events by their
// iCalendar UID field. Submitting the same UID twice overwrites the stored copy
// with identical bytes — the result is unchanged (pure idempotency).
//
// Non-blocking: the handler runs inline (each item is a tiny in-memory write to
// the FS store) and returns 202 with counts. The OS caller fires-and-forgets in
// a background goroutine.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-vcard"

	"github.com/vul-os/vulos-mail/caldav"
	"github.com/vul-os/vulos-mail/carddav"
)

// registerImportHandlers wires the two bulk import endpoints onto mux. The
// handlers are broker-gated and use the same shared contactStore / calStore
// that the CardDAV and CalDAV servers write to, so OS-imported contacts and
// events appear immediately in a calendar client or address book client syncing
// against the same server.
func registerImportHandlers(
	mux *http.ServeMux,
	contactStore carddav.Store,
	calStore caldav.Store,
	brokerSecret string,
) {
	// Exact patterns take precedence over the /api/ subtree handler registered
	// above, regardless of registration order, so no ordering sensitivity here.
	mux.HandleFunc("/api/admin/import/contacts", importContactsHandler(contactStore, brokerSecret))
	mux.HandleFunc("/api/admin/import/events", importEventsHandler(calStore, brokerSecret))
}

// importContactsHandler serves POST /api/admin/import/contacts.
//
// Request body (JSON):
//
//	{
//	  "account": "alice@vulos.to",          // CardDAV account to write into
//	  "vcards":  ["BEGIN:VCARD\r\n...", ...]  // raw vCard 3.0 or 4.0 strings
//	}
//
// Response 202 (JSON):
//
//	{"imported": N, "errors": N}
//
// Each vCard must carry a UID field; the href used for storage is
// "<uid>.vcf" so re-submitting the same UID is a safe overwrite. vCards
// without a UID are rejected (errors++).
func importContactsHandler(store carddav.Store, brokerSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !brokerAuthorized(r, brokerSecret) {
			writeJSONErr(w, http.StatusUnauthorized, "broker authentication required")
			return
		}
		var req struct {
			Account string   `json:"account"`
			VCards  []string `json:"vcards"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "bad request: "+err.Error())
			return
		}
		if strings.TrimSpace(req.Account) == "" {
			writeJSONErr(w, http.StatusBadRequest, "account is required")
			return
		}

		imported, errors := 0, 0
		for _, vc := range req.VCards {
			data := []byte(vc)
			uid, err := vcardUID(data)
			if err != nil || uid == "" {
				errors++
				continue
			}
			href := pimSafeHref(uid) + ".vcf"
			if _, err := store.Put(req.Account, href, data); err != nil {
				errors++
				continue
			}
			imported++
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]int{
			"imported": imported,
			"errors":   errors,
		})
	}
}

// importEventsHandler serves POST /api/admin/import/events.
//
// Request body (JSON):
//
//	{
//	  "account": "alice@vulos.to",           // CalDAV account to write into
//	  "events":  ["BEGIN:VCALENDAR\r\n...", ...]  // one VCALENDAR per element
//	}
//
// Response 202 (JSON):
//
//	{"imported": N, "errors": N}
//
// Each VCALENDAR must contain exactly one VEVENT with a UID. The storage href
// is "<uid>.ics"; re-submitting the same UID is a safe overwrite. Events
// without a parseable UID are rejected (errors++).
func importEventsHandler(store caldav.Store, brokerSecret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONErr(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		if !brokerAuthorized(r, brokerSecret) {
			writeJSONErr(w, http.StatusUnauthorized, "broker authentication required")
			return
		}
		var req struct {
			Account string   `json:"account"`
			Events  []string `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONErr(w, http.StatusBadRequest, "bad request: "+err.Error())
			return
		}
		if strings.TrimSpace(req.Account) == "" {
			writeJSONErr(w, http.StatusBadRequest, "account is required")
			return
		}

		imported, errors := 0, 0
		for _, ev := range req.Events {
			data := []byte(ev)
			uid, err := icalUID(data)
			if err != nil || uid == "" {
				errors++
				continue
			}
			href := pimSafeHref(uid) + ".ics"
			store.Put(req.Account, href, data) // always succeeds for MemStore/FSStore
			imported++
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]int{
			"imported": imported,
			"errors":   errors,
		})
	}
}

// vcardUID extracts the UID property from a raw vCard body. Returns an error
// when the body is not a parseable vCard.
func vcardUID(data []byte) (string, error) {
	dec := vcard.NewDecoder(bytes.NewReader(data))
	card, err := dec.Decode()
	if err != nil {
		return "", fmt.Errorf("vcard parse: %w", err)
	}
	uid := card.Value(vcard.FieldUID)
	return strings.TrimSpace(uid), nil
}

// icalUID extracts the UID property of the first VEVENT in a VCALENDAR body.
// Returns an error when the body is not parseable iCalendar.
func icalUID(data []byte) (string, error) {
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return "", fmt.Errorf("ical parse: %w", err)
	}
	for _, child := range cal.Children {
		if child.Name != ical.CompEvent {
			continue
		}
		ev := &ical.Event{Component: child}
		uid, err := ev.Props.Text(ical.PropUID)
		if err != nil {
			return "", fmt.Errorf("ical: VEVENT has no UID")
		}
		return strings.TrimSpace(uid), nil
	}
	return "", fmt.Errorf("ical: no VEVENT found")
}

// pimSafeHref sanitizes a UID or similar string into a safe filename component.
// It keeps ASCII letters, digits, hyphens, underscores, and dots; everything
// else becomes an underscore. The result is capped at 200 runes.
func pimSafeHref(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i >= 200 {
			break
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}
