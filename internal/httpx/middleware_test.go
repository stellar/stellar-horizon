package httpx

import (
	"net/http"
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
