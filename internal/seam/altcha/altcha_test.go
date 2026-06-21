package altcha

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// solve brute-forces the proof-of-work number and returns the base64 solution.
func solve(t *testing.T, c Challenge) string {
	t.Helper()
	for n := 0; n <= c.MaxNumber; n++ {
		sum := sha256.Sum256([]byte(c.Salt + strconv.Itoa(n)))
		if hex.EncodeToString(sum[:]) == c.Challenge {
			b, _ := json.Marshal(map[string]any{
				"algorithm": c.Algorithm, "challenge": c.Challenge,
				"number": n, "salt": c.Salt, "signature": c.Signature,
			})
			return base64.StdEncoding.EncodeToString(b)
		}
	}
	t.Fatal("no PoW solution found")
	return ""
}

func TestAltchaRoundTripAndReplay(t *testing.T) {
	g := New([]byte("server-secret"), 1000)
	c, err := g.Issue()
	if err != nil {
		t.Fatal(err)
	}
	sol := solve(t, c)
	if err := g.Verify(context.Background(), sol, "1.2.3.4"); err != nil {
		t.Fatalf("valid solution rejected: %v", err)
	}
	// replay of the same solution must fail
	if err := g.Verify(context.Background(), sol, "1.2.3.4"); err == nil {
		t.Fatal("replayed solution accepted")
	}
}

func TestAltchaRejectsForgedSignature(t *testing.T) {
	g := New([]byte("server-secret"), 1000)
	c, _ := g.Issue()
	sol := solve(t, c)
	// Decode, tamper the signature, re-encode.
	raw, _ := base64.StdEncoding.DecodeString(sol)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["signature"] = "00deadbeef"
	b, _ := json.Marshal(m)
	if err := g.Verify(context.Background(), base64.StdEncoding.EncodeToString(b), ""); err == nil {
		t.Fatal("forged signature accepted")
	}
}

func TestAltchaRejectsWrongSolution(t *testing.T) {
	g := New([]byte("server-secret"), 1000)
	c, _ := g.Issue()
	sol := solve(t, c)
	raw, _ := base64.StdEncoding.DecodeString(sol)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	m["number"] = m["number"].(float64) + 1 // wrong pre-image
	b, _ := json.Marshal(m)
	if err := g.Verify(context.Background(), base64.StdEncoding.EncodeToString(b), ""); err == nil {
		t.Fatal("wrong PoW number accepted")
	}
}

func TestAltchaRejectsExpired(t *testing.T) {
	g := New([]byte("server-secret"), 1000)
	c, _ := g.Issue()
	sol := solve(t, c)
	g.now = func() time.Time { return time.Now().Add(g.ttl + time.Minute) } // jump past expiry
	if err := g.Verify(context.Background(), sol, ""); err == nil {
		t.Fatal("expired challenge accepted")
	}
}
