package monitor

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"noroshi/internal/storage"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func gatherMetric(t *testing.T, m *Metrics, name string) *dto.MetricFamily {
	t.Helper()
	families, err := m.registry.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

func TestMetricsRecordCheck(t *testing.T) {
	m := NewMetrics()
	m.RecordCheck("api", true, 150*time.Millisecond)
	m.RecordCheck("api", true, 250*time.Millisecond)
	m.RecordCheck("api", false, 50*time.Millisecond)

	f := gatherMetric(t, m, "noroshi_checks_total")
	if f == nil {
		t.Fatal("noroshi_checks_total not found")
	}
	var upCount, downCount float64
	for _, metric := range f.GetMetric() {
		for _, l := range metric.GetLabel() {
			if l.GetName() == "up" {
				if l.GetValue() == "true" {
					upCount = metric.GetCounter().GetValue()
				} else {
					downCount = metric.GetCounter().GetValue()
				}
			}
		}
	}
	if upCount != 2 || downCount != 1 {
		t.Errorf("checks_total up=%v down=%v, want 2/1", upCount, downCount)
	}

	f = gatherMetric(t, m, "noroshi_check_latency_seconds")
	if f == nil || len(f.GetMetric()) != 1 || f.GetMetric()[0].GetHistogram().GetSampleCount() != 3 {
		t.Errorf("latency histogram should have 3 samples: %+v", f)
	}

	f = gatherMetric(t, m, "noroshi_endpoint_up")
	if f == nil || len(f.GetMetric()) != 1 || f.GetMetric()[0].GetGauge().GetValue() != 0 {
		t.Errorf("endpoint_up should be 0 after a failing last check: %+v", f)
	}
}

func TestMetricsEndpointInfoLifecycle(t *testing.T) {
	m := NewMetrics()
	m.SetEndpointInfo("api", "https://example.com")
	m.SetEndpointInfo("db", "tcp://db:5432")

	f := gatherMetric(t, m, "noroshi_endpoint_info")
	if f == nil || len(f.GetMetric()) != 2 {
		t.Fatalf("endpoint_info should have 2 series: %+v", f)
	}
	var tcpType string
	for _, metric := range f.GetMetric() {
		labels := map[string]string{}
		for _, l := range metric.GetLabel() {
			labels[l.GetName()] = l.GetValue()
		}
		if labels["endpoint"] == "db" {
			tcpType = labels["type"]
		}
	}
	if tcpType != "tcp" {
		t.Errorf("db type label = %q, want tcp", tcpType)
	}

	m.RemoveEndpoint("db")
	f = gatherMetric(t, m, "noroshi_endpoint_info")
	if f == nil || len(f.GetMetric()) != 1 {
		t.Errorf("endpoint_info should have 1 series after removal: %+v", f)
	}
}

func TestMetricsHandler(t *testing.T) {
	m := NewMetrics()
	m.RecordCheck("api", true, time.Millisecond)

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "noroshi_checks_total") {
		t.Errorf("/metrics output missing noroshi_checks_total:\n%s", body)
	}
}

func TestSchedulerRecordsMetrics(t *testing.T) {
	store := newMockStore()
	store.SetEndpoint(storage.Endpoint{ID: 1, Name: "api", URL: "https://example.com", IntervalSeconds: 30, Status: "ok"})
	notifier := &mockNotifier{}
	checker := &mockChecker{
		checkFn: func(_ context.Context, _ string, _ CheckOptions) CheckResult {
			return CheckResult{Up: true, StatusCode: 200, Latency: 10 * time.Millisecond}
		},
	}

	sched := newMockScheduler(t, store, checker, notifier, 3)
	metrics := NewMetrics()
	sched.SetMetrics(metrics)

	if err := sched.Add(context.Background(), storage.Endpoint{ID: 1, Name: "api", URL: "https://example.com", IntervalSeconds: 30}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sched.checkAndNotify(1)

	f := gatherMetric(t, metrics, "noroshi_checks_total")
	if f == nil || len(f.GetMetric()) == 0 {
		t.Fatal("checkAndNotify should record metrics")
	}
	if gatherMetric(t, metrics, "noroshi_endpoint_info") == nil {
		t.Error("Add should register endpoint_info")
	}
}

// Guard against the global-registry anti-pattern: two Metrics instances must
// not collide.
func TestMetricsIsolatedRegistries(t *testing.T) {
	a := NewMetrics()
	b := NewMetrics()
	a.RecordCheck("api", true, time.Millisecond)
	if gatherMetric(t, b, "noroshi_checks_total") != nil {
		t.Error("registries should be isolated")
	}
	_ = prometheus.DefaultRegisterer // referenced to document the intent
}
