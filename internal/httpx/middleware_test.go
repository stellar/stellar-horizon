package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestGzipForDecodingClients(t *testing.T) {
	for _, tc := range []struct {
		userAgent string
		header    []string // Accept-Encoding values sent, nil when the header is absent
		want      []string // Accept-Encoding values the next handler sees
	}{
		{"axios/0.21.4", nil, []string{"gzip"}},
		{"GuzzleHttp/7", nil, []string{"gzip"}},
		{"axios/0.21.4", []string{""}, []string{""}},
		{"GuzzleHttp/7", []string{"br"}, []string{"br"}},
		{"axios/1.6.0", nil, nil},
		{"Go-http-client/1.1", nil, nil},
	} {
		r, err := http.NewRequest(http.MethodGet, "/", nil)
		assert.NoError(t, err)
		r.Header.Set("User-Agent", tc.userAgent)
		if tc.header != nil {
			r.Header["Accept-Encoding"] = tc.header
		}
		var got []string
		gzipForDecodingClients(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = r.Header["Accept-Encoding"]
		})).ServeHTTP(httptest.NewRecorder(), r)
		assert.Equal(t, tc.want, got, "%s %q", tc.userAgent, tc.header)
	}
}
