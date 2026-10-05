package httpx

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewHTTPServerSetsWriteAndIdleDeadlines(t *testing.T) {
	srv := newHTTPServer(":0", http.NewServeMux(), ServerConfig{})
	assert.Positive(t, srv.ReadTimeout)
	assert.Positive(t, srv.WriteTimeout)
	assert.Positive(t, srv.IdleTimeout)
}

func TestNewHTTPServerHonorsConfiguredTimeouts(t *testing.T) {
	cfg := ServerConfig{WriteTimeout: 7 * time.Second, IdleTimeout: 11 * time.Second}
	srv := newHTTPServer(":0", http.NewServeMux(), cfg)
	assert.Equal(t, 7*time.Second, srv.WriteTimeout)
	assert.Equal(t, 11*time.Second, srv.IdleTimeout)
}
