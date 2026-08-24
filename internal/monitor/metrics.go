package monitor

import (
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds the Prometheus instrumentation for check results. It uses
// its own registry (not the global default) so tests are isolated.
//
// The scheduler is the only writer: RecordCheck is called from the
// scheduler's recordCheck choke point (scheduled and ad-hoc checks), and
// SetEndpointInfo/RemoveEndpoint from job add/remove.
type Metrics struct {
	registry *prometheus.Registry
	checks   *prometheus.CounterVec
	latency  *prometheus.HistogramVec
	up       *prometheus.GaugeVec
	info     *prometheus.GaugeVec
}

// NewMetrics creates and registers all noroshi_* metrics.
func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		checks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "noroshi",
			Name:      "checks_total",
			Help:      "Total health checks performed, by endpoint and outcome.",
		}, []string{"endpoint", "up"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "noroshi",
			Name:      "check_latency_seconds",
			Help:      "Health check latency in seconds, by endpoint.",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		}, []string{"endpoint"}),
		up: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "noroshi",
			Name:      "endpoint_up",
			Help:      "Last check outcome by endpoint (1 = up, 0 = down).",
		}, []string{"endpoint"}),
		info: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "noroshi",
			Name:      "endpoint_info",
			Help:      "Static endpoint metadata (always 1) for join queries.",
		}, []string{"endpoint", "url", "type"}),
	}
	m.registry.MustRegister(m.checks, m.latency, m.up, m.info)
	return m
}

// RecordCheck records one check outcome for an endpoint.
func (m *Metrics) RecordCheck(name string, up bool, latency time.Duration) {
	upLabel := "false"
	upValue := 0.0
	if up {
		upLabel = "true"
		upValue = 1
	}
	m.checks.WithLabelValues(name, upLabel).Inc()
	m.latency.WithLabelValues(name).Observe(latency.Seconds())
	m.up.WithLabelValues(name).Set(upValue)
}

// SetEndpointInfo publishes the static endpoint metadata gauge.
// The check type is derived from the URL scheme (http if none).
func (m *Metrics) SetEndpointInfo(name, url string) {
	checkType, _, _ := strings.Cut(url, "://")
	if checkType == "" {
		checkType = "http"
	}
	m.info.WithLabelValues(name, url, checkType).Set(1)
}

// RemoveEndpoint deletes all metric series for an endpoint.
func (m *Metrics) RemoveEndpoint(name string) {
	m.checks.DeletePartialMatch(prometheus.Labels{"endpoint": name})
	m.latency.DeletePartialMatch(prometheus.Labels{"endpoint": name})
	m.up.DeletePartialMatch(prometheus.Labels{"endpoint": name})
	m.info.DeletePartialMatch(prometheus.Labels{"endpoint": name})
}

// Handler returns the Prometheus scrape handler for this registry.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}
