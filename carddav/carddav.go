// Package carddav implements a minimal RFC 6352 CardDAV server exposed as an
// http.Handler. It serves per-account address books of vCard resources over a
// small subset of WebDAV: PROPFIND on a collection, PUT/GET/DELETE on
// individual ".vcf" resources, and a REPORT "addressbook-query" that returns
// the full contact listing. Authentication is HTTP Basic, delegated to an
// injected Auth callback; storage is delegated to a Store. vCard bodies are
// parsed and validated with emersion/go-vcard, and ETags are content hashes.
//
// Scope: a correct read + write path with full-listing reports. Property
// filtering for addressbook-query and richer WebDAV semantics (locking, sync
// collections, ctag) are later refinements.
package carddav

import (
	"crypto/sha256"
	"encoding/hex"
)

// etag returns a strong ETag value (quoted) computed as the SHA-256 hash of the
// resource body. Equal bytes always yield the same ETag.
func etag(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:]) + `"`
}
