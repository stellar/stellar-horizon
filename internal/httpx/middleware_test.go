package httpx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/support/log"
	"github.com/stellar/stellar-horizon/internal/ledger"
	"github.com/stellar/stellar-horizon/internal/render"
)

func TestMethodLabel(t *testing.T) {
	for _, method := range []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace,
	} {
		assert.Equal(t, method, methodLabel(method))
	}
	for _, method := range []string{"get", "Post", "PROPFIND", "PURGE", "X"} {
		assert.Equal(t, "OTHER", methodLabel(method))
	}
}

// TestRouterDoesNotRetainPerRequestState serves requests with varied methods,
// paths, query strings and headers. Once every combination of metric label
// values has been seen, serving more requests should not add metric series,
// retain heap memory or leave goroutines running.
func TestRouterDoesNotRetainPerRequestState(t *testing.T) {
	log.SetLevel(log.WarnLevel)
	t.Cleanup(func() { log.SetLevel(log.InfoLevel) })

	registry := prometheus.NewRegistry()
	srv, err := NewServer(ServerConfig{}, RouterConfig{
		ConnectionTimeout:  time.Minute,
		PrometheusRegistry: registry,
		HealthCheck:        http.NotFoundHandler(),
	}, &ledger.State{})
	require.NoError(t, err)
	srv.RegisterMetrics(registry)

	// An empty method is replaced by a different non-standard method on every request.
	methods := []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace, "",
	}
	// Formatted with the request number, so every request has a new path or query string.
	targets := []string{"/health?cursor=%d", "/unknown/%[1]d?cursor=%[1]d"}
	accepts := []string{"", "application/json", render.MimeEventStream}

	var n uint64
	serveRound := func() {
		for _, method := range methods {
			for _, target := range targets {
				for _, accept := range accepts {
					n++
					m := method
					if m == "" {
						m = fmt.Sprintf("M%d", n)
					}
					req := httptest.NewRequestWithContext(t.Context(), m, fmt.Sprintf(target, n), nil)
					req.Header.Set("Accept", accept)
					req.Header.Set("Origin", fmt.Sprintf("https://%d.example.com", n))
					req.Header.Set("X-Client-Name", fmt.Sprintf("client-%d", n))
					req.Header.Set("X-App-Name", fmt.Sprintf("app-%d", n))
					req.Header.Set("X-Forwarded-For", fmt.Sprintf("10.0.%d.%d", n/256%256, n%256))
					srv.Router.ServeHTTP(httptest.NewRecorder(), req)
				}
			}
		}
	}

	// Warm up so that every label combination and any lazily initialized state exists.
	serveRound()
	serveRound()
	series := countMetricSeries(t, registry)
	heap := liveHeapBytes()
	goroutines := runtime.NumGoroutine()
	start := n

	for range 100 {
		serveRound()
	}

	assert.Equal(t, series, countMetricSeries(t, registry))
	assert.Less(t, liveHeapBytes(), heap+(n-start)*1024, "heap grew by more than 1KiB per request")
	assert.LessOrEqual(t, runtime.NumGoroutine(), goroutines)
}

func countMetricSeries(t *testing.T, registry *prometheus.Registry) int {
	families, err := registry.Gather()
	require.NoError(t, err)
	count := 0
	for _, family := range families {
		count += len(family.GetMetric())
	}
	return count
}

func liveHeapBytes() uint64 {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}
