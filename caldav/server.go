package caldav

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"time"
)

// Backend is a CalDAV server. It pairs an authentication function with a
// storage backend and produces an http.Handler.
type Backend struct {
	// Auth validates HTTP Basic credentials and returns the account
	// identifier whose calendar collection the request operates on. ok is
	// false when authentication fails.
	Auth func(user, pass string) (accountID string, ok bool)
	// Store holds the iCalendar resources.
	Store Store
}

// New creates a Backend with the given auth function and store.
func New(auth func(user, pass string) (string, bool), store Store) *Backend {
	return &Backend{Auth: auth, Store: store}
}

// pathPrefix is the URL space served by the handler. Requests are routed as
// /dav/calendars/{account}/[resource.ics].
const pathPrefix = "/dav/calendars/"

// Handler returns the http.Handler implementing the CalDAV server.
func (b *Backend) Handler() http.Handler {
	return http.HandlerFunc(b.serveHTTP)
}

// serveHTTP authenticates the request, resolves the account and resource, and
// dispatches to the appropriate verb handler.
func (b *Backend) serveHTTP(w http.ResponseWriter, r *http.Request) {
	account, ok := b.authenticate(w, r)
	if !ok {
		return
	}

	path := strings.TrimPrefix(r.URL.Path, pathPrefix)
	if path == r.URL.Path { // prefix not present
		http.NotFound(w, r)
		return
	}
	// path is "{account}" or "{account}/" (collection) or
	// "{account}/resource.ics".
	parts := strings.SplitN(path, "/", 2)
	urlAccount := parts[0]
	if urlAccount != account {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	href := ""
	if len(parts) == 2 {
		href = parts[1]
	}

	w.Header().Set("DAV", "1, calendar-access")

	if href == "" {
		// Collection-level request.
		switch r.Method {
		case "PROPFIND":
			b.propfind(w, r, account)
		case "REPORT":
			b.report(w, r, account)
		case "OPTIONS":
			b.options(w)
		default:
			http.Error(w, "method not allowed on collection", http.StatusMethodNotAllowed)
		}
		return
	}

	// Resource-level request.
	switch r.Method {
	case http.MethodGet:
		b.get(w, account, href)
	case http.MethodPut:
		b.put(w, r, account, href)
	case http.MethodDelete:
		b.del(w, account, href)
	case "REPORT":
		// A REPORT may be addressed at a resource, but calendar-query is a
		// collection report; handle it the same way for robustness.
		b.report(w, r, account)
	case "OPTIONS":
		b.options(w)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// authenticate validates HTTP Basic credentials. On failure it writes a 401
// challenge and returns ok=false.
func (b *Backend) authenticate(w http.ResponseWriter, r *http.Request) (string, bool) {
	user, pass, hasAuth := r.BasicAuth()
	if !hasAuth || b.Auth == nil {
		challenge(w)
		return "", false
	}
	account, ok := b.Auth(user, pass)
	if !ok {
		challenge(w)
		return "", false
	}
	return account, true
}

// challenge writes a Basic auth challenge with a 401 status.
func challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="caldav"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

// options advertises supported methods and CalDAV capabilities.
func (b *Backend) options(w http.ResponseWriter) {
	w.Header().Set("Allow", "OPTIONS, GET, PUT, DELETE, PROPFIND, REPORT")
	w.Header().Set("DAV", "1, calendar-access")
	w.WriteHeader(http.StatusOK)
}

// get returns the raw iCalendar bytes for a resource.
func (b *Backend) get(w http.ResponseWriter, account, href string) {
	ics, ok := b.Store.Get(account, href)
	if !ok {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("ETag", ETag(ics))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(ics)
}

// put stores an iCalendar resource. The body is validated as iCalendar before
// being persisted.
func (b *Backend) put(w http.ResponseWriter, r *http.Request, account, href string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	if !validICS(body) {
		http.Error(w, "invalid iCalendar", http.StatusBadRequest)
		return
	}
	_, existed := b.Store.Get(account, href)
	etag := b.Store.Put(account, href, body)
	w.Header().Set("ETag", etag)
	if existed {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusCreated)
	}
}

// del removes a resource.
func (b *Backend) del(w http.ResponseWriter, account, href string) {
	if !b.Store.Delete(account, href) {
		http.NotFound(w, nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// hrefPath builds the absolute URL path for a resource within an account.
func hrefPath(account, href string) string {
	return pathPrefix + account + "/" + href
}

// multiStatus is the root <D:multistatus> element of a 207 response.
type multiStatus struct {
	XMLName   xml.Name      `xml:"DAV: multistatus"`
	Responses []davResponse `xml:"DAV: response"`
}

// davResponse is a single <D:response> entry.
type davResponse struct {
	Href     string        `xml:"DAV: href"`
	PropStat []davPropStat `xml:"DAV: propstat"`
}

// davPropStat groups properties with a shared status.
type davPropStat struct {
	Prop   davProp `xml:"DAV: prop"`
	Status string  `xml:"DAV: status"`
}

// davProp carries the property values returned for a resource.
type davProp struct {
	DisplayName    string        `xml:"DAV: displayname,omitempty"`
	GetETag        string        `xml:"DAV: getetag,omitempty"`
	GetContentType string        `xml:"DAV: getcontenttype,omitempty"`
	ResourceType   *resourceType `xml:"DAV: resourcetype,omitempty"`
	CalendarData   string        `xml:"urn:ietf:params:xml:ns:caldav calendar-data,omitempty"`
}

// resourceType expresses the <D:resourcetype> property.
type resourceType struct {
	Collection *struct{} `xml:"DAV: collection,omitempty"`
	Calendar   *struct{} `xml:"urn:ietf:params:xml:ns:caldav calendar,omitempty"`
}

// propfind lists the resources in the account's calendar collection. It
// returns a 207 Multi-Status with one response per resource plus one for the
// collection itself.
func (b *Backend) propfind(w http.ResponseWriter, r *http.Request, account string) {
	_, _ = io.Copy(io.Discard, io.LimitReader(r.Body, 1<<20))

	ms := multiStatus{}
	// The collection itself.
	ms.Responses = append(ms.Responses, davResponse{
		Href: pathPrefix + account + "/",
		PropStat: []davPropStat{{
			Status: "HTTP/1.1 200 OK",
			Prop: davProp{
				DisplayName:  account,
				ResourceType: &resourceType{Collection: &struct{}{}, Calendar: &struct{}{}},
			},
		}},
	})

	depth := r.Header.Get("Depth")
	if depth == "" || depth != "0" {
		for _, res := range b.Store.List(account) {
			ms.Responses = append(ms.Responses, davResponse{
				Href: hrefPath(account, res.Href),
				PropStat: []davPropStat{{
					Status: "HTTP/1.1 200 OK",
					Prop: davProp{
						DisplayName:    res.Href,
						GetETag:        res.ETag,
						GetContentType: "text/calendar; charset=utf-8",
						ResourceType:   &resourceType{},
					},
				}},
			})
		}
	}

	writeMultiStatus(w, ms)
}

// calendarQuery models the subset of the calendar-query REPORT request body we
// support: a single comp-filter on VCALENDAR/VEVENT with an optional
// time-range.
type calendarQuery struct {
	XMLName xml.Name `xml:"urn:ietf:params:xml:ns:caldav calendar-query"`
	Filter  struct {
		CompFilter compFilter `xml:"urn:ietf:params:xml:ns:caldav comp-filter"`
	} `xml:"urn:ietf:params:xml:ns:caldav filter"`
}

// compFilter is a (possibly nested) component filter with an optional
// time-range.
type compFilter struct {
	Name       string        `xml:"name,attr"`
	TimeRange  *xmlTimeRange `xml:"urn:ietf:params:xml:ns:caldav time-range"`
	CompFilter *compFilter   `xml:"urn:ietf:params:xml:ns:caldav comp-filter"`
}

// xmlTimeRange is the <C:time-range> element with iCalendar UTC timestamps.
type xmlTimeRange struct {
	Start string `xml:"start,attr"`
	End   string `xml:"end,attr"`
}

// report handles a calendar-query REPORT. It applies any time-range filter
// (expanding recurring events) and returns matching resources together with
// their calendar-data.
func (b *Backend) report(w http.ResponseWriter, r *http.Request, account string) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))

	var q calendarQuery
	tr := parseRequestTimeRange(body, &q)

	ms := multiStatus{}
	for _, res := range b.Store.List(account) {
		if tr != nil && !matchesTimeRange(res.Data, *tr) {
			continue
		}
		ms.Responses = append(ms.Responses, davResponse{
			Href: hrefPath(account, res.Href),
			PropStat: []davPropStat{{
				Status: "HTTP/1.1 200 OK",
				Prop: davProp{
					GetETag:      res.ETag,
					CalendarData: string(res.Data),
				},
			}},
		})
	}

	writeMultiStatus(w, ms)
}

// parseRequestTimeRange extracts the time-range from a calendar-query body,
// returning nil when no usable range is present (in which case all resources
// match). It searches the nested comp-filter chain for the first time-range.
func parseRequestTimeRange(body []byte, q *calendarQuery) *timeRange {
	if err := xml.Unmarshal(body, q); err != nil {
		return nil
	}
	cf := &q.Filter.CompFilter
	for cf != nil {
		if cf.TimeRange != nil {
			tr := &timeRange{}
			if cf.TimeRange.Start != "" {
				if t, err := parseICALTime(cf.TimeRange.Start); err == nil {
					tr.Start = t
				}
			}
			if cf.TimeRange.End != "" {
				if t, err := parseICALTime(cf.TimeRange.End); err == nil {
					tr.End = t
				}
			}
			if tr.Start.IsZero() && tr.End.IsZero() {
				return nil
			}
			return tr
		}
		cf = cf.CompFilter
	}
	return nil
}

// parseICALTime parses an iCalendar UTC timestamp of the form 20060102T150405Z
// (also accepting the floating/basic form without the trailing Z).
func parseICALTime(s string) (time.Time, error) {
	if t, err := time.Parse("20060102T150405Z", s); err == nil {
		return t, nil
	}
	return time.Parse("20060102T150405", s)
}

// writeMultiStatus marshals and writes a 207 Multi-Status response.
func writeMultiStatus(w http.ResponseWriter, ms multiStatus) {
	out, err := xml.Marshal(ms)
	if err != nil {
		http.Error(w, "encoding error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", `application/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(w, xml.Header)
	_, _ = w.Write(out)
}
