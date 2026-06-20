package scan

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/vul-os/vulos-mail/internal/filter"
)

// Rspamd scans a message via an rspamd HTTP daemon (/checkv2). It maps rspamd's
// action/score to a verdict: "reject" → Reject; "add header"/"rewrite subject"
// or score ≥ JunkScore → Junk; else Accept. Errors fail open (Accept) so a down
// scanner never blocks mail.
type Rspamd struct {
	URL      string
	Client   *http.Client
	JunkScore float64
}

// NewRspamd builds an rspamd scanner pointing at baseURL (e.g.
// "http://localhost:11333").
func NewRspamd(baseURL string, junkScore float64) *Rspamd {
	return &Rspamd{
		URL:       strings.TrimRight(baseURL, "/"),
		Client:    &http.Client{Timeout: 5 * time.Second},
		JunkScore: junkScore,
	}
}

func (r *Rspamd) Name() string { return "rspamd" }

type rspamdResp struct {
	Action string  `json:"action"`
	Score  float64 `json:"score"`
}

func (r *Rspamd) Scan(ctx context.Context, raw []byte) filter.Verdict {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.URL+"/checkv2", bytes.NewReader(raw))
	if err != nil {
		return filter.Verdict{Action: filter.Accept}
	}
	req.Header.Set("Content-Type", "message/rfc822")
	resp, err := r.Client.Do(req)
	if err != nil {
		return filter.Verdict{Action: filter.Accept} // fail open
	}
	defer resp.Body.Close()
	var out rspamdResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return filter.Verdict{Action: filter.Accept}
	}
	switch strings.ToLower(out.Action) {
	case "reject":
		return filter.Verdict{Action: filter.Reject, Reason: "rspamd-reject"}
	case "add header", "rewrite subject", "soft reject":
		return filter.Verdict{Action: filter.Junk, Reason: "rspamd-spam"}
	}
	if r.JunkScore > 0 && out.Score >= r.JunkScore {
		return filter.Verdict{Action: filter.Junk, Reason: "rspamd-score"}
	}
	return filter.Verdict{Action: filter.Accept}
}
