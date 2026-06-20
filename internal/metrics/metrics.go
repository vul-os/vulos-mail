// Package metrics is the Prometheus instrumentation for vulos-mail (ported from
// vulos-mail's metrics/obs). Collectors live here as package vars on a private
// registry; the app layer (Manager, scheduler loop, adapters) increments them,
// keeping the pure internal libraries free of a Prometheus dependency.
package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// reg is a dedicated registry so tests are isolated and we don't pull default
// Go/process collectors unless asked.
var reg = prometheus.NewRegistry()

var (
	// MessagesReceived counts inbound deliveries by disposition (inbox/spam/reject).
	MessagesReceived = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vulos_messages_received_total",
		Help: "Inbound messages by delivery disposition.",
	}, []string{"disposition"})

	// Outbound counts delivery attempts by outcome (delivered/deferred/bounced).
	Outbound = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "vulos_outbound_total",
		Help: "Outbound delivery results by outcome.",
	}, []string{"outcome"})

	// SubmissionsAccepted counts authenticated submissions accepted for delivery.
	SubmissionsAccepted = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "vulos_submissions_accepted_total",
		Help: "Authenticated submissions accepted.",
	})

	// QueueDepth reports the outbound scheduler queue depth.
	QueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "vulos_queue_depth",
		Help: "Outbound scheduler queue depth.",
	})
)

func init() {
	reg.MustRegister(MessagesReceived, Outbound, SubmissionsAccepted, QueueDepth)
}

// Handler returns the /metrics HTTP handler exposing the vulos-mail registry.
func Handler() http.Handler {
	return promhttp.HandlerFor(reg, promhttp.HandlerOpts{})
}
