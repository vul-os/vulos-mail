// Package altcha implements an Altcha-compatible proof-of-work signup gate — a
// self-hosted, privacy-respecting anti-abuse challenge with NO external service
// (https://altcha.org). The server issues a signed challenge; the client must
// find the pre-image number; the server verifies the work, the signature (so the
// challenge can't be forged), the expiry, and rejects replays.
package altcha

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Gate is an Altcha proof-of-work SignupGate.
type Gate struct {
	secret    []byte
	maxNumber int
	ttl       time.Duration
	now       func() time.Time

	mu   sync.Mutex
	used map[string]time.Time // challenge -> first-seen (replay protection)
}

// New returns a Gate signing challenges with secret. maxNumber bounds the search
// space (higher = more client work); 0 picks a sensible default.
func New(secret []byte, maxNumber int) *Gate {
	if maxNumber <= 0 {
		maxNumber = 100_000
	}
	return &Gate{secret: secret, maxNumber: maxNumber, ttl: 10 * time.Minute, now: time.Now, used: map[string]time.Time{}}
}

// Challenge is the JSON payload handed to the client.
type Challenge struct {
	Algorithm string `json:"algorithm"`
	Challenge string `json:"challenge"`
	Salt      string `json:"salt"`
	Signature string `json:"signature"`
	MaxNumber int    `json:"maxnumber"`
}

// Issue mints a fresh, signed, time-limited challenge.
func (g *Gate) Issue() (Challenge, error) {
	saltRaw := make([]byte, 12)
	if _, err := rand.Read(saltRaw); err != nil {
		return Challenge{}, err
	}
	expires := g.now().Add(g.ttl).Unix()
	salt := fmt.Sprintf("%s?expires=%d", hex.EncodeToString(saltRaw), expires)

	nb := make([]byte, 8)
	if _, err := rand.Read(nb); err != nil {
		return Challenge{}, err
	}
	secretNumber := int(binary.BigEndian.Uint64(nb) % uint64(g.maxNumber+1))

	sum := sha256.Sum256([]byte(salt + strconv.Itoa(secretNumber)))
	challenge := hex.EncodeToString(sum[:])
	return Challenge{
		Algorithm: "SHA-256",
		Challenge: challenge,
		Salt:      salt,
		Signature: g.sign(challenge),
		MaxNumber: g.maxNumber,
	}, nil
}

// IssueJSON returns a freshly-issued challenge as JSON (for the HTTP handler).
func (g *Gate) IssueJSON() ([]byte, error) {
	c, err := g.Issue()
	if err != nil {
		return nil, err
	}
	return json.Marshal(c)
}

func (g *Gate) sign(challenge string) string {
	mac := hmac.New(sha256.New, g.secret)
	mac.Write([]byte(challenge))
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify implements seam.SignupGate: it decodes the base64 Altcha solution and
// checks signature, expiry, proof-of-work, and replay.
func (g *Gate) Verify(_ context.Context, solution, _ string) error {
	raw, err := base64.StdEncoding.DecodeString(solution)
	if err != nil {
		return errors.New("altcha: malformed solution")
	}
	var p struct {
		Algorithm string `json:"algorithm"`
		Challenge string `json:"challenge"`
		Number    int    `json:"number"`
		Salt      string `json:"salt"`
		Signature string `json:"signature"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return errors.New("altcha: malformed solution")
	}
	if p.Algorithm != "SHA-256" {
		return errors.New("altcha: unsupported algorithm")
	}
	// Signature proves WE issued this challenge (constant-time compare).
	want, _ := hex.DecodeString(g.sign(p.Challenge))
	got, err := hex.DecodeString(p.Signature)
	if err != nil || !hmac.Equal(got, want) {
		return errors.New("altcha: bad signature")
	}
	// Expiry encoded in the salt.
	if exp := expiresFrom(p.Salt); exp > 0 && g.now().Unix() > exp {
		return errors.New("altcha: challenge expired")
	}
	// The actual proof of work.
	sum := sha256.Sum256([]byte(p.Salt + strconv.Itoa(p.Number)))
	if hex.EncodeToString(sum[:]) != p.Challenge {
		return errors.New("altcha: invalid solution")
	}
	// Replay protection (one solution per challenge).
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gcLocked()
	if _, seen := g.used[p.Challenge]; seen {
		return errors.New("altcha: solution already used")
	}
	g.used[p.Challenge] = g.now()
	return nil
}

func (g *Gate) gcLocked() {
	cutoff := g.now().Add(-g.ttl)
	for c, t := range g.used {
		if t.Before(cutoff) {
			delete(g.used, c)
		}
	}
}

func expiresFrom(salt string) int64 {
	i := strings.Index(salt, "?expires=")
	if i < 0 {
		return 0
	}
	v := salt[i+len("?expires="):]
	if j := strings.IndexByte(v, '&'); j >= 0 {
		v = v[:j]
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}
