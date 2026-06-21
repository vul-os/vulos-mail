package cloud_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	cloud "github.com/vul-os/vulos-mail/integration/cloud"
	"github.com/vul-os/vulos-mail/internal/seam"
)

func TestDisabledWhenNoURL(t *testing.T) {
	if cloud.New("", "secret") != nil {
		t.Fatal("cloud client should be nil when no base URL is configured")
	}
	if cloud.NewIdentity(nil) != nil || cloud.NewEntitlements(nil) != nil || cloud.NewUsage(nil) != nil {
		t.Fatal("adapters should be nil when the client is nil")
	}
}

func TestCloudAdapterAgainstStub(t *testing.T) {
	ctx := context.Background()
	var sawAuth, sawMetered, sawSignup bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Relay-Auth") != "shh" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/api/mail/signup":
			sawSignup = true
			w.WriteHeader(http.StatusOK)
		case "/api/mail/auth":
			sawAuth = true
			_ = json.NewEncoder(w).Encode(map[string]string{"account": "alice@vulos.to"})
		case "/api/entitlements":
			_ = json.NewEncoder(w).Encode(map[string]any{"tier": "pro", "max_send_per_day": 500, "suspended": false})
		case "/api/usage":
			sawMetered = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := cloud.New(srv.URL, "shh")
	id := cloud.NewIdentity(c)

	acct, err := id.Authenticate(ctx, "alice@vulos.to", "pw")
	if err != nil || acct != "alice@vulos.to" || !sawAuth {
		t.Fatalf("authenticate = %q, %v (sawAuth=%v)", acct, err, sawAuth)
	}
	if err := id.Provision(ctx, "newuser@vulos.to", "pw12345678"); err != nil || !sawSignup {
		t.Fatalf("Provision should create a free cp account: err=%v sawSignup=%v", err, sawSignup)
	}

	plan, err := cloud.NewEntitlements(c).For(ctx, "alice@vulos.to")
	if err != nil || plan.Tier != "pro" || plan.MaxSendPerDay != 500 {
		t.Fatalf("entitlements = %+v, %v", plan, err)
	}

	cloud.NewUsage(c).Report(ctx, seam.Event{Kind: "send", Account: "alice@vulos.to", Count: 1})
	if !sawMetered {
		t.Fatal("usage event was not reported to cp")
	}
}
