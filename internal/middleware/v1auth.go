// Package middleware provides HTTP middleware for the vulos-mail /v1 API.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/vul-os/vulos-mail/internal/apikey"
)

// ctxKey is an unexported type for context keys in this package, preventing
// collisions with keys from other packages.
type ctxKey int

const ctxV1Auth ctxKey = 0

// AuthInfo holds the resolved authentication identity set in the request context
// by V1Auth. It is retrieved with GetV1Auth.
type AuthInfo struct {
	// Method is "apikey" when authenticated via a vk_ API key or "session" when
	// authenticated via the existing webmail session cookie.
	Method string
	// Account is the authenticated account email address. For API-key auth this
	// comes from the CP introspection result; for session auth it is the
	// signed-in mailbox address.
	Account string
	// Secret is an opaque value the proxy handler uses to obtain the credentials
	// it needs. For "apikey" it is the raw vk_ key itself (passed to the engine
	// as X-Vulos-Mail-Secret with X-Vulos-Mail-Auth: apikey). For "session" it
	// is the session-store token the handler uses to look up the IMAP password.
	Secret string
}

// GetV1Auth retrieves the AuthInfo set by the V1Auth middleware from ctx.
// It returns (AuthInfo{}, false) if the middleware did not run or auth failed.
func GetV1Auth(ctx context.Context) (AuthInfo, bool) {
	v, ok := ctx.Value(ctxV1Auth).(AuthInfo)
	return v, ok
}

// V1Auth is an http.Handler middleware that gates /v1 routes, accepting EITHER:
//
//   - a Vulos API key — `Authorization: Bearer vk_…` — validated via the cloud
//     introspection seam (apikey.Introspector). The key must be valid and carry
//     the "mail" product scope, OR
//   - the existing webmail session — validated by the caller-supplied sessAuth
//     callback (typically a session-store cookie lookup).
//
// A vk_ key is only attempted when an introspector is wired (intro != nil, i.e.
// VULOS_CP_BASE_URL is configured). When it is NOT configured the key path is
// disabled and only session auth applies — self-host is unchanged.
//
// Unlike the browser webmail, /v1 NEVER redirects: every failure is a JSON
// error body with the appropriate status (401/403/503).
//
// On success, AuthInfo is placed in the request context and retrievable with
// GetV1Auth. API keys never carry the admin scope: a key acts only as its own
// account, never as a tenant-wide admin.
//
// The sessAuth callback is called for any credential that is NOT a vk_ key
// (including when no Authorization header is present at all). It returns the
// account email, an opaque secret for broker use, and whether auth succeeded.
func V1Auth(
	intro apikey.Introspector,
	sessAuth func(r *http.Request) (account, secret string, ok bool),
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)

		// ── API-key path ───────────────────────────────────────────────────────
		// Only when an introspector is configured AND the credential looks like a
		// Vulos API key. A vk_ token is never tried as a session credential (and
		// vice versa), so the two schemes can't be confused.
		if intro != nil && strings.HasPrefix(raw, apikey.KeyPrefix) {
			res, err := intro.Introspect(r.Context(), raw)
			if err != nil {
				// CP unreachable: fail closed rather than guess.
				writeErr(w, http.StatusServiceUnavailable, "API key validation unavailable")
				return
			}
			if !res.Valid {
				writeErr(w, http.StatusUnauthorized, "invalid API key")
				return
			}
			if !res.HasProduct(apikey.ProductMail) {
				writeErr(w, http.StatusForbidden, "API key not authorized for the mail product")
				return
			}
			ctx := context.WithValue(r.Context(), ctxV1Auth, AuthInfo{
				Method:  "apikey",
				Account: res.Account,
				Secret:  raw, // raw vk_ key passed through to the engine broker
			})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// ── Session path ───────────────────────────────────────────────────────
		// Delegate to the caller-supplied session validator (webmail cookie store
		// in the main server). The /v1 proxy always requires real credentials so
		// there is no "auth disabled" mode here: an absent or invalid session is
		// always a 401.
		account, secret, ok := sessAuth(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), ctxV1Auth, AuthInfo{
			Method:  "session",
			Account: account,
			Secret:  secret,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// bearerToken extracts the raw token from an `Authorization: Bearer <token>`
// header (no scheme prefix, trimmed), or "" when absent or malformed.
func bearerToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

// writeErr writes a {"error": msg} JSON body with the given HTTP status.
func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
