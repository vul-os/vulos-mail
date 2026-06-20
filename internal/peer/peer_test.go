package peer

import "testing"

func newTestRegistry() *Registry {
	r := NewRegistry()
	r.AddAll([]string{"acme.com", "Example.ORG"})
	r.Add("widgets.io")
	return r
}

func TestIsPeer(t *testing.T) {
	r := newTestRegistry()

	tests := []struct {
		name   string
		domain string
		want   bool
	}{
		{"exact match", "acme.com", true},
		{"exact match added individually", "widgets.io", true},
		{"subdomain of peer", "mail.acme.com", true},
		{"deep subdomain of peer", "a.b.c.acme.com", true},
		{"case-insensitive exact", "ACME.COM", true},
		{"case-insensitive added mixed-case peer", "example.org", true},
		{"case-insensitive subdomain", "MX.Example.Org", true},
		{"trailing dot fqdn", "acme.com.", true},
		{"non-peer", "evil.com", false},
		{"non-peer sharing suffix label only", "notacme.com", false},
		{"peer name as a subdomain of attacker", "acme.com.evil.com", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.IsPeer(tt.domain); got != tt.want {
				t.Errorf("IsPeer(%q) = %v, want %v", tt.domain, got, tt.want)
			}
		})
	}
}

func TestVerified(t *testing.T) {
	r := newTestRegistry()

	tests := []struct {
		name       string
		fromDomain string
		dmarcPass  bool
		want       bool
	}{
		{"peer and dmarc pass", "acme.com", true, true},
		{"peer subdomain and dmarc pass", "mail.acme.com", true, true},
		{"peer but dmarc fail", "acme.com", false, false},
		{"not peer but dmarc pass", "evil.com", true, false},
		{"not peer and dmarc fail", "evil.com", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := r.Verified(tt.fromDomain, tt.dmarcPass); got != tt.want {
				t.Errorf("Verified(%q, %v) = %v, want %v", tt.fromDomain, tt.dmarcPass, got, tt.want)
			}
		})
	}
}
