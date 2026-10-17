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
		// Responds with the first media type listed that Horizon serves
		{"text/event-stream", MimeEventStream},
		{"text/event-stream,application/hal+json", MimeEventStream},
		{"application/hal+json, text/event-stream", MimeHal},
		{"application/json, text/event-stream", MimeHal},
		{"*/*, text/event-stream", MimeEventStream},
		{"application/octet-stream", MimeRaw},
		// Ignores media range parameters, including q-values
		{"text/event-stream; charset=utf-8", MimeEventStream},
		{"text/event-stream;q=0.5, application/hal+json", MimeEventStream},
		// Defaults to HAL
		{"application/hal+json", MimeHal},
		{"application/json, text/plain, */*", MimeHal},
		{"text/plain", MimeHal},
		{"", MimeHal},
		// Matches whole media types only
		{"text/event-streaming", MimeHal},
		{"application/octet-stream-custom", MimeHal},
		{"application/json;profile=text/event-stream", MimeHal},
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
