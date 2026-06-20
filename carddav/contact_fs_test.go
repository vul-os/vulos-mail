package carddav_test

import (
	"testing"

	"github.com/vul-os/vmail/carddav"
)

func TestContactRoundTrip(t *testing.T) {
	vcf, err := carddav.BuildContact(carddav.Contact{Name: "Dana Okoro", Email: "dana@acme.io"})
	if err != nil {
		t.Fatal(err)
	}
	c := carddav.ParseContact(vcf)
	if c.Name != "Dana Okoro" || c.Email != "dana@acme.io" {
		t.Fatalf("round-trip wrong: %+v", c)
	}
}

func TestFSStorePersistsContacts(t *testing.T) {
	dir := t.TempDir()
	s, err := carddav.NewFSStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	vcf, _ := carddav.BuildContact(carddav.Contact{Name: "Carol", Email: "carol@x.com"})
	if _, err := s.Put("alice@vmail.test", "c1.vcf", vcf); err != nil {
		t.Fatal(err)
	}

	// Reopen from disk: the contact survives.
	s2, _ := carddav.NewFSStore(dir)
	res, err := s2.List("alice@vmail.test")
	if err != nil || len(res) != 1 {
		t.Fatalf("List = %d (%v), want 1", len(res), err)
	}
	if c := carddav.ParseContact(res[0].Data); c.Email != "carol@x.com" {
		t.Errorf("persisted contact wrong: %+v", c)
	}
	if err := s2.Delete("alice@vmail.test", "c1.vcf"); err != nil {
		t.Fatal(err)
	}
	if res, _ := s2.List("alice@vmail.test"); len(res) != 0 {
		t.Errorf("after delete, List = %d, want 0", len(res))
	}
}
