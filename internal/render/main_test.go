package render

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNegotiate(t *testing.T) {
	r, err := http.NewRequest("GET", "/ledgers", nil)
	assert.Nil(t, err)
	r.WithContext(context.Background())

	testCases := []struct {
		Header               string
		ExpectedResponseType string
	}{
		// Streams when the Accept header asks for text/event-stream
		{"text/event-stream", MimeEventStream},
		{"text/event-stream,application/hal+json", MimeEventStream},
		// Responds with raw bytes when it asks for application/octet-stream
		{"application/octet-stream", MimeRaw},
		// Defaults to HAL
		{"application/hal+json", MimeHal},
		{"application/json, text/plain, */*", MimeHal},
		{"text/plain", MimeHal},
		{"", MimeHal},
	}
	for _, tc := range testCases {
		t.Run("", func(t *testing.T) {
			r.Header.Set("Accept", tc.Header)
			assert.Equal(t, tc.ExpectedResponseType, Negotiate(r))
		})
	}

	// Defaults to MimeHal even with no Accept key set
	r.Header.Del("Accept")
	assert.Equal(t, MimeHal, Negotiate(r))
}
