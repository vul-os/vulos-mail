package mtaout

import (
	"fmt"

	"github.com/vul-os/vulos-deliver/backend/ses"
	"github.com/vul-os/vulos-deliver/provider"
)

// SenderConfig controls which outbound-delivery backend vulos-mail uses.
//
// When DeliverBackend is empty (the default) the built-in direct-SMTP path is
// selected, preserving the self-hosted single-binary behaviour: no external
// dependencies, no cloud credentials required.
//
// When DeliverBackend is "ses" or "smtp" the outbound path goes through the
// shared vulos-deliver engine (opt-in, cloud/SaaS deployments only).
type SenderConfig struct {
	// HELO is the hostname announced in the EHLO/HELO greeting for the
	// built-in SMTP sender. Ignored when DeliverBackend is set.
	HELO string

	// STARTTLSEnforce, if true, causes the built-in SMTP sender to fail
	// (TempFail, retriable) when the remote MX does not advertise STARTTLS.
	// Default (false): opportunistic — STARTTLS is negotiated when offered.
	// Set via the STARTTLS_ENFORCE environment variable. Ignored when
	// DeliverBackend is set (managed delivery handles transport security).
	STARTTLSEnforce bool

	// DeliverBackend selects the vulos-deliver backend: "ses" or "smtp".
	// Leave empty to use the built-in direct-SMTP path (default).
	// Set via the DELIVER_BACKEND environment variable.
	DeliverBackend string

	// SES credentials — used when DeliverBackend == "ses".
	// When SESKey/SESSecret are empty, ses.New falls back to the standard
	// AWS credential chain (IAM role, ~/.aws/credentials, etc.).
	SESRegion    string // DELIVER_SES_REGION
	SESKey       string // DELIVER_SES_KEY
	SESSecret    string // DELIVER_SES_SECRET
	SESConfigSet string // DELIVER_SES_CONFIG_SET (optional)
}

// NewSender returns the Sender selected by cfg.
//
// Decision table:
//
//	DeliverBackend == ""        → &SMTPSender (built-in, self-host default)
//	DeliverBackend == "ses"     → DeliverSender wrapping the SES backend
//	DeliverBackend == "smtp"    → DeliverSender wrapping the deliver-SMTP relay
func NewSender(cfg SenderConfig) (Sender, error) {
	if cfg.DeliverBackend == "" {
		// Default: built-in direct-SMTP path. No external deps.
		return &SMTPSender{HELO: cfg.HELO, STARTTLSEnforce: cfg.STARTTLSEnforce}, nil
	}

	pcfg := provider.Config{
		Backend: cfg.DeliverBackend,
		SES: ses.Config{
			Region:           cfg.SESRegion,
			AccessKeyID:      cfg.SESKey,
			SecretAccessKey:  cfg.SESSecret,
			ConfigurationSet: cfg.SESConfigSet,
		},
	}
	d, err := provider.New(pcfg)
	if err != nil {
		return nil, fmt.Errorf("outbound sender (deliver/%s): %w", cfg.DeliverBackend, err)
	}
	return NewDeliverSender(d), nil
}
