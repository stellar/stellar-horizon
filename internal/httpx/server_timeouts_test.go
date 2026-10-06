package httpx

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewHTTPServerSetsWriteAndIdleDeadlines(t *testing.T) {
	srv := newHTTPServer(":0", http.NewServeMux())
	assert.Positive(t, srv.ReadTimeout)
	assert.Positive(t, srv.WriteTimeout)
	assert.Positive(t, srv.IdleTimeout)
}
