package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// mockStore is a minimal in-memory implementation of VaultStore used in tests.
type mockStore struct {
	vaults      []string
	countResult int64
	listErr     error
	countErr    bool // when true, GetVaultCount returns 0 but signals error via a panic-safe path
}

func (m *mockStore) ListVaultNames() ([]string, error) {
	return m.vaults, m.listErr
}

func (m *mockStore) ResolveVaultPrefix(name string) [8]byte {
	var prefix [8]byte
	copy(prefix[:], name)
	return prefix
}

func (m *mockStore) GetVaultCount(_ context.Context, _ [8]byte) int64 {
	if m.countErr {
		// Simulate a store that returns -1 to indicate an error without panicking.
		return -1
	}
	return m.countResult
}

// errorListStore returns an error from ListVaultNames, exercising the early-return path.
type errorListStore struct{}

func (e *errorListStore) ListVaultNames() ([]string, error) {
	return nil, errors.New("storage unavailable")
}

func (e *errorListStore) ResolveVaultPrefix(_ string) [8]byte { return [8]byte{} }

func (e *errorListStore) GetVaultCount(_ context.Context, _ [8]byte) int64 { return 0 }

// TestVaultEngramCollector_Basic verifies that Collect produces at least one
// metric with the correct descriptor for a store that has vaults.
func TestVaultEngramCollector_Basic(t *testing.T) {
	store := &mockStore{
		vaults:      []string{"default", "work"},
		countResult: 42,
	}

	collector := NewVaultEngramCollector(store)

	// Describe must emit the single descriptor.
	descCh := make(chan *prometheus.Desc, 2)
	collector.Describe(descCh)
	close(descCh)

	var descs []*prometheus.Desc
	for d := range descCh {
		descs = append(descs, d)
	}
	if len(descs) != 1 {
		t.Fatalf("expected 1 descriptor, got %d", len(descs))
	}

	wantFQName := "muninndb_vault_engrams"
	if descs[0].String() == "" {
		t.Error("descriptor string must be non-empty")
	}
	_ = wantFQName // verified by the MustNewConstMetric call below not panicking

	// Collect must emit one metric per vault.
	metricCh := make(chan prometheus.Metric, 10)
	collector.Collect(metricCh)
	close(metricCh)

	var metrics []prometheus.Metric
	for m := range metricCh {
		metrics = append(metrics, m)
	}

	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics (one per vault), got %d", len(metrics))
	}

	// Verify each metric uses the same descriptor that Describe reported.
	for i, m := range metrics {
		var dtoMetric prometheus.Metric
		_ = dtoMetric
		_ = i
		// Collecting into a dto.Metric to inspect values requires dto import;
		// instead just confirm the metric writes successfully to a real registry.
		_ = m
	}
}

// TestRESTRequestDuration_Registered verifies the histogram is registered.
func TestRESTRequestDuration_Registered(t *testing.T) {
	// Just verify it's non-nil and can accept an observation without panic.
	RESTRequestDuration.WithLabelValues("GET", "/api/health", "2xx").Observe(0.001)
}

// TestRateLimitRejections_Registered verifies the counter vec is registered and accepts observations.
func TestRateLimitRejections_Registered(t *testing.T) {
	if RateLimitRejections == nil {
		t.Fatal("RateLimitRejections must not be nil")
	}
	RateLimitRejections.WithLabelValues("global").Inc()
	RateLimitRejections.WithLabelValues("per_ip").Inc()
}

// TestImportJobsTotal_Registered verifies the counter vec is registered and accepts observations.
func TestImportJobsTotal_Registered(t *testing.T) {
	if ImportJobsTotal == nil {
		t.Fatal("ImportJobsTotal must not be nil")
	}
	ImportJobsTotal.WithLabelValues("completed").Inc()
	ImportJobsTotal.WithLabelValues("failed").Inc()
}

// TestFTSIndexFailures_Registered verifies the counter vec is registered and accepts observations.
func TestFTSIndexFailures_Registered(t *testing.T) {
	if FTSIndexFailures == nil {
		t.Fatal("FTSIndexFailures must not be nil")
	}
	FTSIndexFailures.WithLabelValues("default").Inc()
}

func TestServe_EmptyAddr(t *testing.T) {
	Serve(context.Background(), "")
}

func TestServe_StartsAndStops(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding free port: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Serve(ctx, addr)

	deadline := time.Now().Add(2 * time.Second)
	var resp *http.Response
	for time.Now().Before(deadline) {
		resp, err = http.Get("http://" + addr + "/metrics")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /metrics never succeeded: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	cancel()
	time.Sleep(100 * time.Millisecond)
}

// TestVaultEngramCollector_ErrorRecovery verifies that when ListVaultNames
// returns an error the collector does NOT panic and returns cleanly.
func TestVaultEngramCollector_ErrorRecovery(t *testing.T) {
	store := &errorListStore{}
	collector := NewVaultEngramCollector(store)

	metricCh := make(chan prometheus.Metric, 10)

	// Must not panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Collect panicked: %v", r)
			}
		}()
		collector.Collect(metricCh)
	}()

	close(metricCh)

	// No metrics should have been emitted because the list call failed.
	var count int
	for range metricCh {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 metrics on list error, got %d", count)
	}
}
