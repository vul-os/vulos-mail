package region_test

import (
	"testing"

	"github.com/vul-os/vulos-mail/internal/region"
)

func TestDefaultResolvesToEU(t *testing.T) {
	r := region.New()
	ep := r.Resolve("alice@example.com")
	if ep.Region != region.EU {
		t.Errorf("default region = %q, want %q", ep.Region, region.EU)
	}
	if ep.URL != "" {
		t.Errorf("phase-0 URL should be empty, got %q", ep.URL)
	}
}

func TestNilResolverResolvesToEU(t *testing.T) {
	var r *region.Resolver
	ep := r.Resolve("alice@example.com")
	if ep.Region != region.EU {
		t.Errorf("nil resolver region = %q, want %q", ep.Region, region.EU)
	}
}

func TestMailboxOverride(t *testing.T) {
	r := region.New()
	r.SetMailbox("bob@us-cell.example.com", "us", "https://us.mail.internal")
	ep := r.Resolve("bob@us-cell.example.com")
	if ep.Region != "us" {
		t.Errorf("mailbox override region = %q, want %q", ep.Region, "us")
	}
	if ep.URL != "https://us.mail.internal" {
		t.Errorf("mailbox override URL = %q, want %q", ep.URL, "https://us.mail.internal")
	}
	// Unrelated mailbox still returns the default.
	ep2 := r.Resolve("carol@other.example.com")
	if ep2.Region != region.EU {
		t.Errorf("unrelated mailbox region = %q, want %q", ep2.Region, region.EU)
	}
}

func TestDomainOverride(t *testing.T) {
	r := region.New()
	r.SetDomain("ap-cell.example.com", "ap", "https://ap.mail.internal")
	// Any mailbox in that domain resolves to ap.
	ep := r.Resolve("alice@ap-cell.example.com")
	if ep.Region != "ap" {
		t.Errorf("domain override region = %q, want %q", ep.Region, "ap")
	}
	// Per-mailbox override beats domain override.
	r.SetMailbox("bob@ap-cell.example.com", "eu", "")
	ep2 := r.Resolve("bob@ap-cell.example.com")
	if ep2.Region != region.EU {
		t.Errorf("mailbox-beats-domain: region = %q, want %q", ep2.Region, region.EU)
	}
}

func TestCaseInsensitive(t *testing.T) {
	r := region.New()
	r.SetMailbox("Alice@Example.COM", "us", "")
	ep := r.Resolve("alice@example.com")
	if ep.Region != "us" {
		t.Errorf("case-insensitive lookup region = %q, want %q", ep.Region, "us")
	}
}
