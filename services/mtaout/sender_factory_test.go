package mtaout_test

import (
	"context"
	"testing"

	deliver "github.com/vul-os/vulos-deliver"
	"github.com/vul-os/vulos-mail/services/mtaout"
)

// mockDeliverBackend is a test double for deliver.Sender.
type mockDeliverBackend struct {
	called bool
	err    error
}

func (m *mockDeliverBackend) Send(_ context.Context, _ deliver.Message) (deliver.Receipt, error) {
	m.called = true
	return deliver.Receipt{Backend: "mock"}, m.err
}

func (m *mockDeliverBackend) SendBatch(_ context.Context, msgs []deliver.Message) ([]deliver.Receipt, error) {
	receipts := make([]deliver.Receipt, len(msgs))
	for i := range msgs {
		receipts[i] = deliver.Receipt{Backend: "mock"}
	}
	return receipts, m.err
}

func (m *mockDeliverBackend) Close() error { return nil }

// TestNewSender_DefaultIsBuiltinSMTP verifies that omitting DeliverBackend
// returns the built-in *SMTPSender, keeping the self-hosted default unchanged.
func TestNewSender_DefaultIsBuiltinSMTP(t *testing.T) {
	s, err := mtaout.NewSender(mtaout.SenderConfig{HELO: "mail.example.com"})
	if err != nil {
		t.Fatalf("NewSender: %v", err)
	}
	if _, ok := s.(*mtaout.SMTPSender); !ok {
		t.Fatalf("expected *SMTPSender when DeliverBackend is empty, got %T", s)
	}
}

// TestDeliverSender_Adapts verifies that DeliverSender correctly bridges
// OutMessage to deliver.Message and returns Delivered on success.
func TestDeliverSender_Adapts(t *testing.T) {
	mock := &mockDeliverBackend{}
	s := mtaout.NewDeliverSender(mock)

	msg := mtaout.OutMessage{
		Tenant:     "tenant1",
		From:       "from@example.com",
		Rcpts:      []string{"to@dest.example"},
		Raw:        []byte("Subject: hi\r\n\r\nbody"),
		RcptDomain: "dest.example",
	}
	res := s.Send(context.Background(), msg, "1.2.3.4") // sourceIP ignored by design
	if res.Status != mtaout.Delivered {
		t.Fatalf("expected Delivered, got %v (err: %v)", res.Status, res.Err)
	}
	if !mock.called {
		t.Fatal("expected deliver.Sender.Send to be called")
	}
}

// TestDeliverSender_ErrSuppressed_IsPermFail verifies that ErrSuppressed from
// the deliver backend maps to PermFail (no point retrying a suppressed address).
func TestDeliverSender_ErrSuppressed_IsPermFail(t *testing.T) {
	mock := &mockDeliverBackend{err: deliver.ErrSuppressed}
	s := mtaout.NewDeliverSender(mock)

	res := s.Send(context.Background(), mtaout.OutMessage{
		From:  "a@b.com",
		Rcpts: []string{"x@y.z"},
		Raw:   []byte("Subject: suppressed\r\n\r\n."),
	}, "")
	if res.Status != mtaout.PermFail {
		t.Fatalf("expected PermFail for ErrSuppressed, got %v", res.Status)
	}
}

// TestDeliverSender_ErrSandboxMode_IsPermFail verifies that ErrSandboxMode
// maps to PermFail (unverified recipient in SES sandbox — no retry useful).
func TestDeliverSender_ErrSandboxMode_IsPermFail(t *testing.T) {
	mock := &mockDeliverBackend{err: deliver.ErrSandboxMode}
	s := mtaout.NewDeliverSender(mock)

	res := s.Send(context.Background(), mtaout.OutMessage{
		From:  "a@b.com",
		Rcpts: []string{"unverified@dest.example"},
		Raw:   []byte("Subject: sandbox\r\n\r\n."),
	}, "")
	if res.Status != mtaout.PermFail {
		t.Fatalf("expected PermFail for ErrSandboxMode, got %v", res.Status)
	}
}

// TestDeliverSender_TransientError_IsTempFail verifies that other errors from
// the backend map to TempFail so the Scheduler retries with backoff.
func TestDeliverSender_TransientError_IsTempFail(t *testing.T) {
	mock := &mockDeliverBackend{err: context.DeadlineExceeded}
	s := mtaout.NewDeliverSender(mock)

	res := s.Send(context.Background(), mtaout.OutMessage{
		From:  "a@b.com",
		Rcpts: []string{"x@y.z"},
		Raw:   []byte("Subject: transient\r\n\r\n."),
	}, "")
	if res.Status != mtaout.TempFail {
		t.Fatalf("expected TempFail for transient error, got %v", res.Status)
	}
}
