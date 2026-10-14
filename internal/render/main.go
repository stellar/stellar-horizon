package render

import (
	"net/http"
	"strings"
)

// Negotiate returns the media type to respond to the provided request with:
// MimeEventStream if its Accept header asks for a stream, MimeRaw if it asks
// for raw bytes, and MimeHal otherwise.
func Negotiate(r *http.Request) string {
	accept := r.Header.Get("Accept")
	switch {
	case strings.Contains(accept, MimeEventStream):
		return MimeEventStream
	case strings.Contains(accept, MimeRaw):
		return MimeRaw
	default:
		return MimeHal
	}
}
