package byo_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/byo"
)

// ---------------------------------------------------------------------------
// Crypto: GenerateKeypair + Encrypt/Decrypt round-trip
// ---------------------------------------------------------------------------

func TestEncryptDecryptRoundtrip(t *testing.T) {
	tests := []struct {
		name      string
		plaintext []byte
	}{
		{"simple", []byte("Hello, BYO mail!")},
		{"multiline", []byte("line one\nline two\nsecret\n")},
		{"empty", []byte("")},
		{"binary", []byte{0x00, 0x01, 0xff, 0xfe, 0x42, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pub, priv, err := byo.GenerateKeypair()
			if err != nil {
				t.Fatalf("GenerateKeypair: %v", err)
			}
			ct, err := byo.Encrypt(pub, tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if len(tt.plaintext) > 0 && bytes.Equal(ct, tt.plaintext) {
				t.Fatal("ciphertext equals plaintext")
			}
			if len(ct) == 0 {
				t.Fatal("expected non-empty ciphertext")
			}
			got, err := byo.Decrypt(priv, ct)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(got, tt.plaintext) {
				t.Errorf("roundtrip mismatch: got %q, want %q", got, tt.plaintext)
			}
		})
	}
}

func TestGenerateKeypairFormat(t *testing.T) {
	pub, priv, err := byo.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	if len(pub) < 4 || pub[:4] != "age1" {
		t.Errorf("public key %q does not start with age1", pub)
	}
	const wantPrefix = "AGE-SECRET-KEY-"
	if len(priv) < len(wantPrefix) || priv[:len(wantPrefix)] != wantPrefix {
		t.Errorf("private key %q does not start with %s", priv, wantPrefix)
	}
	// Two generations must differ.
	pub2, _, _ := byo.GenerateKeypair()
	if pub == pub2 {
		t.Error("two generated public keys are identical")
	}
}

// ---------------------------------------------------------------------------
// Crypto: decrypting with a wrong/different key errors
// ---------------------------------------------------------------------------

func TestDecryptWrongKey(t *testing.T) {
	pub, _, err := byo.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	_, wrongPriv, err := byo.GenerateKeypair()
	if err != nil {
		t.Fatalf("GenerateKeypair: %v", err)
	}
	ct, err := byo.Encrypt(pub, []byte("secret"))
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	tests := []struct {
		name string
		priv string
		ct   []byte
	}{
		{"wrong identity", wrongPriv, ct},
		{"malformed identity", "not-a-valid-key", ct},
		{"corrupt ciphertext", wrongPriv, []byte("garbage")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := byo.Decrypt(tt.priv, tt.ct); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestEncryptBadRecipient(t *testing.T) {
	if _, err := byo.Encrypt("not-a-recipient", []byte("x")); err == nil {
		t.Error("expected error for malformed recipient, got nil")
	}
}

// ---------------------------------------------------------------------------
// Queue: Put / Fetch / Delete semantics
// ---------------------------------------------------------------------------

func TestMemStorePutFetch(t *testing.T) {
	tests := []struct {
		name string
		puts []struct {
			account, msgID string
			ct             []byte
		}
		fetchAccount string
		wantMsgIDs   []string // expected order (ReceivedAt asc, MessageID tiebreak)
	}{
		{
			name: "single put",
			puts: []struct {
				account, msgID string
				ct             []byte
			}{
				{"acc-1", "msg-001", []byte("ct1")},
			},
			fetchAccount: "acc-1",
			wantMsgIDs:   []string{"msg-001"},
		},
		{
			name: "multiple puts same account",
			puts: []struct {
				account, msgID string
				ct             []byte
			}{
				{"acc-1", "msg-b", []byte("b")},
				{"acc-1", "msg-a", []byte("a")},
			},
			fetchAccount: "acc-1",
			wantMsgIDs:   []string{"msg-a", "msg-b"}, // same ReceivedAt -> msgID order
		},
		{
			name: "account isolation",
			puts: []struct {
				account, msgID string
				ct             []byte
			}{
				{"acc-1", "msg-1", []byte("x")},
				{"acc-2", "msg-2", []byte("y")},
			},
			fetchAccount: "acc-2",
			wantMsgIDs:   []string{"msg-2"},
		},
		{
			name:         "empty account",
			fetchAccount: "nobody",
			wantMsgIDs:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := byo.NewMemStore()
			// Fix the clock so same-time entries are deterministic.
			fixed := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
			s.SetNow(func() time.Time { return fixed })
			for _, p := range tt.puts {
				s.Put(p.account, p.msgID, p.ct)
			}
			got := s.Fetch(tt.fetchAccount)
			if len(got) != len(tt.wantMsgIDs) {
				t.Fatalf("Fetch len = %d, want %d", len(got), len(tt.wantMsgIDs))
			}
			for i, want := range tt.wantMsgIDs {
				if got[i].MessageID != want {
					t.Errorf("entry[%d].MessageID = %q, want %q", i, got[i].MessageID, want)
				}
				if got[i].AccountID != tt.fetchAccount {
					t.Errorf("entry[%d].AccountID = %q, want %q", i, got[i].AccountID, tt.fetchAccount)
				}
				if !got[i].ReceivedAt.Equal(fixed) {
					t.Errorf("entry[%d].ReceivedAt = %v, want %v", i, got[i].ReceivedAt, fixed)
				}
			}
		})
	}
}

func TestMemStoreFetchReturnsCiphertext(t *testing.T) {
	s := byo.NewMemStore()
	ct := []byte("encrypted-blob")
	s.Put("acc", "m1", ct)
	got := s.Fetch("acc")
	if len(got) != 1 {
		t.Fatalf("Fetch len = %d, want 1", len(got))
	}
	if !bytes.Equal(got[0].Ciphertext, ct) {
		t.Errorf("Ciphertext = %q, want %q", got[0].Ciphertext, ct)
	}
	// Put must take a defensive copy.
	ct[0] = 'X'
	got2 := s.Fetch("acc")
	if bytes.Equal(got2[0].Ciphertext, ct) {
		t.Error("stored ciphertext mutated when caller mutated input slice")
	}
}

func TestMemStoreDelete(t *testing.T) {
	s := byo.NewMemStore()
	s.Put("acc", "m1", []byte("a"))
	s.Put("acc", "m2", []byte("b"))

	s.Delete("acc", "m1")
	got := s.Fetch("acc")
	if len(got) != 1 {
		t.Fatalf("after delete, Fetch len = %d, want 1", len(got))
	}
	if got[0].MessageID != "m2" {
		t.Errorf("remaining MessageID = %q, want m2", got[0].MessageID)
	}

	// Deleting a non-existent entry is a no-op.
	s.Delete("acc", "does-not-exist")
	s.Delete("ghost-account", "m2")
	if len(s.Fetch("acc")) != 1 {
		t.Error("no-op delete altered store")
	}

	// Delete the last entry: account becomes empty.
	s.Delete("acc", "m2")
	if got := s.Fetch("acc"); got != nil {
		t.Errorf("after deleting all, Fetch = %v, want nil", got)
	}
}

func TestMemStorePutOverwrites(t *testing.T) {
	s := byo.NewMemStore()
	s.Put("acc", "m1", []byte("first"))
	s.Put("acc", "m1", []byte("second"))
	got := s.Fetch("acc")
	if len(got) != 1 {
		t.Fatalf("Fetch len = %d, want 1 (overwrite)", len(got))
	}
	if !bytes.Equal(got[0].Ciphertext, []byte("second")) {
		t.Errorf("Ciphertext = %q, want second", got[0].Ciphertext)
	}
}

func TestMemStoreConcurrent(t *testing.T) {
	s := byo.NewMemStore()
	const n = 100
	done := make(chan struct{})
	for i := 0; i < n; i++ {
		go func(i int) {
			defer func() { done <- struct{}{} }()
			id := string(rune('A'+i%26)) + time.Now().String()
			s.Put("acc", id, []byte("ct"))
			_ = s.Fetch("acc")
			s.Delete("acc", id)
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}
	// No assertion beyond completing without the race detector tripping.
}
