package carddav

import (
	"encoding/xml"
	"net/http"
	"strings"
)

// XML namespaces used in WebDAV / CardDAV multistatus documents.
const (
	nsDAV     = "DAV:"
	nsCardDAV = "urn:ietf:params:xml:ns:carddav"
	statusOK  = "HTTP/1.1 200 OK"
)

// multistatus is the root <D:multistatus> element of a 207 response.
type multistatus struct {
	XMLName   xml.Name      `xml:"D:multistatus"`
	XMLNSD    string        `xml:"xmlns:D,attr"`
	XMLNSC    string        `xml:"xmlns:C,attr"`
	Responses []davResponse `xml:"D:response"`
}

// davResponse is a single <D:response> describing one href.
type davResponse struct {
	Href     string        `xml:"D:href"`
	Propstat []davPropstat `xml:"D:propstat"`
}

// davPropstat groups properties that share an HTTP status.
type davPropstat struct {
	Prop   davProp `xml:"D:prop"`
	Status string  `xml:"D:status"`
}

// davProp is the set of properties returned for a resource. Empty fields are
// omitted so a collection and a vCard resource can share the type.
type davProp struct {
	DisplayName    string        `xml:"D:displayname,omitempty"`
	GetETag        string        `xml:"D:getetag,omitempty"`
	GetContentType string        `xml:"D:getcontenttype,omitempty"`
	ResourceType   *resourceType `xml:"D:resourcetype,omitempty"`
	AddressData    string        `xml:"C:address-data,omitempty"`
}

// resourceType expresses <D:resourcetype>. Collections set Collection and (for
// address books) Addressbook; plain resources leave it empty.
type resourceType struct {
	Collection  *struct{} `xml:"D:collection,omitempty"`
	Addressbook *struct{} `xml:"C:addressbook,omitempty"`
}

// newMultistatus returns a multistatus with namespaces pre-populated.
func newMultistatus() *multistatus {
	return &multistatus{XMLNSD: nsDAV, XMLNSC: nsCardDAV}
}

// handlePropfind lists the address book collection and its member resources.
// It honours the Depth header (0 = collection only, otherwise collection plus
// members) and always responds 207 Multi-Status.
func (b *Backend) handlePropfind(w http.ResponseWriter, r *http.Request, account string) {
	depth := r.Header.Get("Depth")

	ms := newMultistatus()
	collHref := pathPrefix + account + "/"
	collType := &resourceType{Collection: &struct{}{}, Addressbook: &struct{}{}}
	ms.Responses = append(ms.Responses, davResponse{
		Href: collHref,
		Propstat: []davPropstat{{
			Prop: davProp{
				DisplayName:  account,
				ResourceType: collType,
			},
			Status: statusOK,
		}},
	})

	if depth != "0" {
		resources, err := b.Store.List(account)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, res := range resources {
			ms.Responses = append(ms.Responses, davResponse{
				Href: collHref + res.Href,
				Propstat: []davPropstat{{
					Prop: davProp{
						DisplayName:    res.Href,
						GetETag:        res.ETag,
						GetContentType: "text/vcard; charset=utf-8",
						ResourceType:   &resourceType{},
					},
					Status: statusOK,
				}},
			})
		}
	}

	writeMultistatus(w, ms)
}

// handleReport implements the addressbook-query REPORT. Filtering is not yet
// applied: it returns every contact in the account's address book, including
// each contact's vCard data via <C:address-data>.
func (b *Backend) handleReport(w http.ResponseWriter, r *http.Request, account string) {
	// The addressbook-query filter is intentionally ignored for now: we return
	// the full listing. Property filtering is a later refinement. (The request
	// body is closed by the server after the handler returns.)
	resources, err := b.Store.List(account)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	ms := newMultistatus()
	collHref := pathPrefix + account + "/"
	for _, res := range resources {
		ms.Responses = append(ms.Responses, davResponse{
			Href: collHref + res.Href,
			Propstat: []davPropstat{{
				Prop: davProp{
					GetETag:     res.ETag,
					AddressData: string(res.Data),
				},
				Status: statusOK,
			}},
		})
	}

	writeMultistatus(w, ms)
}

// writeMultistatus marshals ms and writes a 207 Multi-Status response.
func writeMultistatus(w http.ResponseWriter, ms *multistatus) {
	var buf strings.Builder
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(ms); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusMultiStatus)
	_, _ = w.Write([]byte(buf.String()))
}
