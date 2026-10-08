package httpx

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewHTTPServerDerivesWriteTimeoutFromConnectionTimeout(t *testing.T) {
	connectionTimeout := 90 * time.Second
	srv := newHTTPServer(":0", http.NewServeMux(), connectionTimeout)
	assert.Equal(t, connectionTimeout+writeTimeoutBuffer, srv.WriteTimeout)
	assert.Positive(t, srv.ReadTimeout)
	assert.Positive(t, srv.IdleTimeout)
}

func TestNewHTTPServerFallsBackWhenConnectionTimeoutUnset(t *testing.T) {
	srv := newHTTPServer(":0", http.NewServeMux(), 0)
	assert.Equal(t, defaultWriteTimeout, srv.WriteTimeout)
}
