package render

import (
	"net/http"
	"strings"
)

// Negotiate returns the media type to respond to the provided request with:
// the first media type listed in its Accept header that Horizon serves
// (MimeEventStream, MimeRaw, or MimeHal, which also covers application/json),
// or MimeHal if it lists none. Media range parameters, including q-values, are
// ignored.
func Negotiate(r *http.Request) string {
	for mediaRange := range strings.SplitSeq(r.Header.Get("Accept"), ",") {
		mediaType, _, _ := strings.Cut(mediaRange, ";")
		switch mediaType = strings.TrimSpace(mediaType); mediaType {
		case MimeEventStream, MimeRaw:
			return mediaType
		case MimeHal, "application/json":
			return MimeHal
		}
	}
	return MimeHal
}
