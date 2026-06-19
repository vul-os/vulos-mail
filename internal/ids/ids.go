// Package ids generates sortable ULID-style identifiers. The generator takes an
// injectable clock and randomness source so tests can produce deterministic ids
// (the spine's determinism invariants depend on this being controllable).
package ids

import (
	"crypto/rand"
	"io"
	"sync"
	"time"
)

// Crockford base32 alphabet (ULID spec).
const enc = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// Gen produces ULIDs. Safe for concurrent use.
type Gen struct {
	mu   sync.Mutex
	now  func() time.Time
	rand io.Reader
}

// NewGen returns a production generator (wall clock + crypto/rand).
func NewGen() *Gen { return &Gen{now: time.Now, rand: rand.Reader} }

// NewGenWith returns a generator with injected clock and randomness, for tests.
func NewGenWith(now func() time.Time, r io.Reader) *Gen {
	return &Gen{now: now, rand: r}
}

// New returns a 26-char ULID: 48-bit big-endian millisecond timestamp followed
// by 80 bits of randomness, Crockford base32 encoded. Lexicographically sortable
// by time.
func (g *Gen) New() string {
	g.mu.Lock()
	defer g.mu.Unlock()

	var b [16]byte
	ms := uint64(g.now().UnixMilli())
	b[0] = byte(ms >> 40)
	b[1] = byte(ms >> 32)
	b[2] = byte(ms >> 24)
	b[3] = byte(ms >> 16)
	b[4] = byte(ms >> 8)
	b[5] = byte(ms)
	_, _ = io.ReadFull(g.rand, b[6:])
	return encode(b)
}

func encode(b [16]byte) string {
	var s [26]byte
	s[0] = enc[(b[0]&224)>>5]
	s[1] = enc[b[0]&31]
	s[2] = enc[(b[1]&248)>>3]
	s[3] = enc[((b[1]&7)<<2)|((b[2]&192)>>6)]
	s[4] = enc[(b[2]&62)>>1]
	s[5] = enc[((b[2]&1)<<4)|((b[3]&240)>>4)]
	s[6] = enc[((b[3]&15)<<1)|((b[4]&128)>>7)]
	s[7] = enc[(b[4]&124)>>2]
	s[8] = enc[((b[4]&3)<<3)|((b[5]&224)>>5)]
	s[9] = enc[b[5]&31]
	s[10] = enc[(b[6]&248)>>3]
	s[11] = enc[((b[6]&7)<<2)|((b[7]&192)>>6)]
	s[12] = enc[(b[7]&62)>>1]
	s[13] = enc[((b[7]&1)<<4)|((b[8]&240)>>4)]
	s[14] = enc[((b[8]&15)<<1)|((b[9]&128)>>7)]
	s[15] = enc[(b[9]&124)>>2]
	s[16] = enc[((b[9]&3)<<3)|((b[10]&224)>>5)]
	s[17] = enc[b[10]&31]
	s[18] = enc[(b[11]&248)>>3]
	s[19] = enc[((b[11]&7)<<2)|((b[12]&192)>>6)]
	s[20] = enc[(b[12]&62)>>1]
	s[21] = enc[((b[12]&1)<<4)|((b[13]&240)>>4)]
	s[22] = enc[((b[13]&15)<<1)|((b[14]&128)>>7)]
	s[23] = enc[(b[14]&124)>>2]
	s[24] = enc[((b[14]&3)<<3)|((b[15]&224)>>5)]
	s[25] = enc[b[15]&31]
	return string(s[:])
}
