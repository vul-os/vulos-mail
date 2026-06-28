package mtaout

import (
	"context"
	"errors"

	deliver "github.com/vul-os/vulos-deliver"
)

// DeliverSender adapts a deliver.Sender (e.g. the SES backend from
// vulos-deliver) to the mtaout.Sender interface.
//
// The sourceIP parameter is intentionally ignored: cloud-provider transports
// (SES, managed SMTP relay) control egress IPs transparently, so the pool-
// assigned IP does not apply. The Scheduler's pool/warmup/reputation logic
// continues to run normally; only the final network hop changes.
type DeliverSender struct {
	backend deliver.Sender
}

// NewDeliverSender wraps d as a mtaout.Sender ready for use by the Scheduler.
func NewDeliverSender(d deliver.Sender) *DeliverSender {
	return &DeliverSender{backend: d}
}

// Send adapts an OutMessage to deliver.Message and dispatches it through the
// vulos-deliver backend.
//
// Status mapping:
//   - nil error               → Delivered
//   - ErrSuppressed / ErrSandboxMode → PermFail (no point retrying)
//   - any other error         → TempFail (API throttle, network, etc.)
func (s *DeliverSender) Send(ctx context.Context, msg OutMessage, _ string) SendResult {
	dm := deliver.Message{
		TenantID: msg.Tenant,
		From:     deliver.Address{Email: msg.From},
		MIMEBody: msg.Raw,
	}
	for _, r := range msg.Rcpts {
		dm.To = append(dm.To, deliver.Address{Email: r})
	}

	_, err := s.backend.Send(ctx, dm)
	if err == nil {
		return SendResult{Status: Delivered}
	}
	if errors.Is(err, deliver.ErrSuppressed) || errors.Is(err, deliver.ErrSandboxMode) {
		return SendResult{Status: PermFail, Err: err}
	}
	return SendResult{Status: TempFail, Err: err}
}
