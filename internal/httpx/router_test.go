package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/stellar-horizon/internal/ledger"
)

func newTestServer(t *testing.T, maxConcurrentRequests uint, healthCheck http.HandlerFunc) *Server {
	server, err := NewServer(ServerConfig{}, RouterConfig{
		MaxConcurrentRequests: maxConcurrentRequests,
		ConnectionTimeout:     time.Minute,
		PrometheusRegistry:    prometheus.NewRegistry(),
		HealthCheck:           healthCheck,
	}, &ledger.State{})
	require.NoError(t, err)
	return server
}

func TestRouterRecordsRoutePattern(t *testing.T) {
	server := newTestServer(t, 0, func(w http.ResponseWriter, r *http.Request) {})

	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var received dto.Metric
	require.NoError(t, server.Metrics.RequestsReceivedCounter.WithLabelValues("/health", "false", http.MethodGet).Write(&received))
	assert.Equal(t, 1.0, received.GetCounter().GetValue())
}

func TestRouterRejectsRequestsOverConcurrencyLimit(t *testing.T) {
	inHandler := make(chan struct{})
	release := make(chan struct{})
	server := newTestServer(t, 1, func(w http.ResponseWriter, r *http.Request) {
		close(inHandler)
		<-release
	})

	firstStatus := make(chan int)
	go func() {
		w := httptest.NewRecorder()
		server.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
		firstStatus <- w.Code
	}()
	<-inHandler

	w := httptest.NewRecorder()
	server.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/not-found", nil))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)

	close(release)
	assert.Equal(t, http.StatusOK, <-firstStatus)
}
