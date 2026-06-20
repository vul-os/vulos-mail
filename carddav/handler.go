package carddav

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/emersion/go-vcard"
)

// pathPrefix is the URL space the Handler routes under. Resource URLs look like
// /dav/addressbooks/{account}/{href}.
const pathPrefix = "/dav/addressbooks/"

// Auth authenticates an HTTP Basic credential pair. It returns the account
// identifier the credentials map to (which scopes the address book) and ok=true
// on success, or ok=false to reject the request with 401.
type Auth func(user, pass string) (accountID string, ok bool)

// Backend is a CardDAV server. It pairs an Auth strategy with a Store and
// produces an http.Handler. The zero value is not usable; construct one with
// the desired fields populated.
type Backend struct {
	// Auth authenticates requests. It must be non-nil.
	Auth Auth
	// Store persists address book resources. It must be non-nil.
	Store Store
}

// Handler returns an http.Handler serving the CardDAV endpoints under
// /dav/addressbooks/. Requests outside that prefix receive 404.
func (b *Backend) Handler() http.Handler {
	return http.HandlerFunc(b.serve)
}

// serve is the single entry point: it authenticates, then dispatches on whether
// the request targets a collection or an individual resource.
func (b *Backend) serve(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, pathPrefix) {
		http.NotFound(w, r)
		return
	}

	user, pass, hasAuth := r.BasicAuth()
	if !hasAuth {
		unauthorized(w)
		return
	}
	account, ok := b.Auth(user, pass)
	if !ok {
		unauthorized(w)
		return
	}

	// Split {account}/{href...} from the routed prefix.
	rest := strings.TrimPrefix(r.URL.Path, pathPrefix)
	urlAccount, href, _ := strings.Cut(rest, "/")
	if urlAccount == "" {
		http.NotFound(w, r)
		return
	}
	// An authenticated account may only touch its own address book.
	if urlAccount != account {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	href = strings.Trim(href, "/")
	collection := href == ""

	switch r.Method {
	case "PROPFIND":
		b.handlePropfind(w, r, account)
	case "REPORT":
		b.handleReport(w, r, account)
	case http.MethodGet:
		if collection {
			http.Error(w, "method not allowed on collection", http.StatusMethodNotAllowed)
			return
		}
		b.handleGet(w, r, account, href)
	case http.MethodPut:
		if collection {
			http.Error(w, "method not allowed on collection", http.StatusMethodNotAllowed)
			return
		}
		b.handlePut(w, r, account, href)
	case http.MethodDelete:
		if collection {
			http.Error(w, "method not allowed on collection", http.StatusMethodNotAllowed)
			return
		}
		b.handleDelete(w, r, account, href)
	case "OPTIONS":
		w.Header().Set("DAV", "1, 3, addressbook")
		w.Header().Set("Allow", "OPTIONS, GET, PUT, DELETE, PROPFIND, REPORT")
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGet writes the raw vCard body of a single resource.
func (b *Backend) handleGet(w http.ResponseWriter, r *http.Request, account, href string) {
	res, err := b.Store.Get(account, href)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("ETag", res.ETag)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(res.Data)
}

// handlePut validates the request body as a vCard and stores it. A new resource
// yields 201 Created; replacing an existing one yields 204 No Content.
func (b *Backend) handlePut(w http.ResponseWriter, r *http.Request, account, href string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := validateVCard(body); err != nil {
		http.Error(w, "invalid vCard: "+err.Error(), http.StatusBadRequest)
		return
	}

	_, existsErr := b.Store.Get(account, href)
	existed := existsErr == nil

	res, err := b.Store.Put(account, href, body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("ETag", res.ETag)
	if existed {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

// handleDelete removes a single resource.
func (b *Backend) handleDelete(w http.ResponseWriter, r *http.Request, account, href string) {
	err := b.Store.Delete(account, href)
	if errors.Is(err, ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateVCard parses body with go-vcard and confirms it contains at least one
// well-formed card. An empty or malformed body is rejected.
func validateVCard(body []byte) error {
	dec := vcard.NewDecoder(bytes.NewReader(body))
	card, err := dec.Decode()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("empty body")
		}
		return err
	}
	if len(card) == 0 {
		return errors.New("vCard has no fields")
	}
	return nil
}

// unauthorized writes a 401 response with a Basic challenge.
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="carddav"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
