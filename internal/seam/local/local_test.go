package local_test

import (
	"context"
	"testing"

	seamlocal "github.com/vul-os/vulos-mail/internal/seam/local"
)

func TestLocalIdentityProvisionAuthPersist(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	id, err := seamlocal.NewIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := id.Provision(ctx, "Alice@Vulos.to", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	// case-insensitive existence + auth
	if !id.Exists("alice@vulos.to") {
		t.Fatal("provisioned account should exist")
	}
	if acct, err := id.Authenticate(ctx, "ALICE@vulos.to", "correct horse battery"); err != nil || acct != "alice@vulos.to" {
		t.Fatalf("auth = %q, %v", acct, err)
	}
	if _, err := id.Authenticate(ctx, "alice@vulos.to", "wrong"); err == nil {
		t.Fatal("wrong password accepted")
	}
	// duplicate provision rejected
	if err := id.Provision(ctx, "alice@vulos.to", "x"); err == nil {
		t.Fatal("duplicate provision accepted")
	}

	// persistence: a fresh store over the same dir sees the account
	id2, err := seamlocal.NewIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !id2.Exists("alice@vulos.to") {
		t.Fatal("account did not persist across reopen")
	}
	if _, err := id2.Authenticate(ctx, "alice@vulos.to", "correct horse battery"); err != nil {
		t.Fatalf("auth after reopen: %v", err)
	}

	// Upsert overwrites the password
	if err := id2.Upsert("alice@vulos.to", "new-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := id2.Authenticate(ctx, "alice@vulos.to", "new-pass-123"); err != nil {
		t.Fatalf("auth after upsert: %v", err)
	}
	if id2.Count() != 1 {
		t.Fatalf("Count = %d, want 1", id2.Count())
	}
}
