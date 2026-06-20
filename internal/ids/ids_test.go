package ids_test

import (
	"bytes"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vul-os/vulos-mail/internal/ids"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// valid checks an id is a 26-char Crockford base32 string (ULID shape).
func valid(t *testing.T, id string) {
	t.Helper()
	if len(id) != 26 {
		t.Fatalf("id %q has length %d, want 26", id, len(id))
	}
	for i, c := range id {
		if !strings.ContainsRune(crockford, c) {
			t.Fatalf("id %q has invalid char %q at %d", id, c, i)
		}
	}
}

// TestShapeAndNonEmpty verifies a production generator emits well-formed ids.
func TestShapeAndNonEmpty(t *testing.T) {
	g := ids.NewGen()
	for i := 0; i < 1000; i++ {
		valid(t, g.New())
	}
}

// TestUniqueness checks ids do not collide across many calls. Even within the
// same millisecond the 80 random bits make collisions astronomically unlikely.
func TestUniqueness(t *testing.T) {
	g := ids.NewGen()
	const n = 100_000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := g.New()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id generated: %q", id)
		}
		seen[id] = struct{}{}
	}
}

// TestMonotonicAcrossTime verifies that as the clock advances, ids sort
// lexicographically in time order — the property the log spine depends on.
func TestMonotonicAcrossTime(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	step := time.Duration(0)
	clock := func() time.Time { return base.Add(step) }
	// Fixed randomness keeps the timestamp prefix the sole differentiator.
	g := ids.NewGenWith(clock, bytes.NewReader(bytes.Repeat([]byte{0xAB}, 16*1000)))

	var prev string
	for i := 0; i < 1000; i++ {
		step = time.Duration(i) * time.Millisecond
		id := g.New()
		valid(t, id)
		if prev != "" && id <= prev {
			t.Fatalf("id not monotonic: step %d gave %q <= prev %q", i, id, prev)
		}
		prev = id
	}
}

// TestSortableMatchesGeneration confirms that sorting a batch generated with an
// advancing clock yields the original generation order.
func TestSortableMatchesGeneration(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	step := time.Duration(0)
	clock := func() time.Time { return base.Add(step) }
	g := ids.NewGenWith(clock, bytes.NewReader(bytes.Repeat([]byte{0x01}, 16*200)))

	var gen []string
	for i := 0; i < 200; i++ {
		step = time.Duration(i) * time.Second
		gen = append(gen, g.New())
	}
	sorted := append([]string(nil), gen...)
	sort.Strings(sorted)
	for i := range gen {
		if gen[i] != sorted[i] {
			t.Fatalf("at %d: generation order %q != sorted order %q", i, gen[i], sorted[i])
		}
	}
}

// TestDeterministicWithFixedClockAndRand confirms the injected sources fully
// determine output (required by the spine's determinism invariants).
func TestDeterministicWithFixedClockAndRand(t *testing.T) {
	fixed := time.Unix(1_700_000_000, 123)
	clk := func() time.Time { return fixed }
	g1 := ids.NewGenWith(clk, bytes.NewReader(bytes.Repeat([]byte{0x7F}, 10)))
	g2 := ids.NewGenWith(clk, bytes.NewReader(bytes.Repeat([]byte{0x7F}, 10)))
	if a, b := g1.New(), g2.New(); a != b {
		t.Fatalf("same clock+rand produced different ids: %q vs %q", a, b)
	}
}

// TestConcurrentSafety races many goroutines through one generator. Run with
// `go test -race ./internal/ids/` to exercise the mutex.
func TestConcurrentSafety(t *testing.T) {
	g := ids.NewGen()
	const goroutines = 16
	const perG = 5000

	var (
		mu  sync.Mutex
		all = make(map[string]struct{}, goroutines*perG)
		wg  sync.WaitGroup
	)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := make([]string, 0, perG)
			for j := 0; j < perG; j++ {
				local = append(local, g.New())
			}
			mu.Lock()
			for _, id := range local {
				all[id] = struct{}{}
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	if got, want := len(all), goroutines*perG; got != want {
		t.Fatalf("collisions under concurrency: %d unique ids, want %d", got, want)
	}
}
