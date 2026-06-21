package server_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/vul-os/vulos-mail/internal/model"
)

// Hammer the single-writer runtime with many concurrent deliveries and reads.
// Run with -race: catches data races and proves every message is durably folded
// in (the projection stays consistent under concurrency).
func TestConcurrentDeliveryStress(t *testing.T) {
	ctx := context.Background()
	m := newMgr(t)
	_ = m.AddAccount("alice@vulos.to", "pw")

	const N = 200
	var wg sync.WaitGroup
	errs := make(chan error, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			raw := []byte(fmt.Sprintf("From: s%d@x\r\nTo: alice@vulos.to\r\nSubject: msg-%d\r\n\r\nbody %d\r\n", i, i, i))
			if err := m.Deliver(ctx, "alice@vulos.to", raw); err != nil {
				errs <- err
			}
		}(i)
	}
	// Concurrent readers racing the writers (must not panic / race).
	for r := 0; r < 8; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rt, err := m.AuthIMAP("alice@vulos.to", "pw")
			if err != nil {
				return
			}
			for k := 0; k < 50; k++ {
				_ = rt.MessagesWithLabel(model.LabelInbox)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent delivery failed: %v", err)
	}

	rt, _ := m.AuthIMAP("alice@vulos.to", "pw")
	got := rt.MessagesWithLabel(model.LabelInbox)
	if len(got) != N {
		t.Fatalf("after %d concurrent deliveries, inbox has %d", N, len(got))
	}
}
