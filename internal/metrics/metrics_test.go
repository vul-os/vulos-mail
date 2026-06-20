package metrics_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vul-os/vmail/internal/metrics"
)

func TestHandlerExposesCounters(t *testing.T) {
	metrics.MessagesReceived.WithLabelValues("inbox").Inc()
	metrics.Outbound.WithLabelValues("delivered").Inc()
	metrics.SubmissionsAccepted.Inc()
	metrics.QueueDepth.Set(7)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	metrics.Handler().ServeHTTP(rec, req)

	body, _ := io.ReadAll(rec.Result().Body)
	out := string(body)
	for _, want := range []string{
		"vmail_messages_received_total",
		`disposition="inbox"`,
		"vmail_outbound_total",
		"vmail_submissions_accepted_total",
		"vmail_queue_depth 7",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("/metrics missing %q", want)
		}
	}
}
