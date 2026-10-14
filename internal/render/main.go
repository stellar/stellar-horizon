package render

import (
	"net/http"
	"strings"
)

// Negotiate returns the media type to respond to the provided request with:
// MimeEventStream if its Accept header asks for a stream, MimeRaw if it asks
// for raw bytes, and MimeHal otherwise. Whichever of the two media types is
// listed first wins, and media range parameters, including q-values, are
// ignored.
func Negotiate(r *http.Request) string {
	for mediaRange := range strings.SplitSeq(r.Header.Get("Accept"), ",") {
		mediaType, _, _ := strings.Cut(mediaRange, ";")
		mediaType = strings.TrimSpace(mediaType)
		if mediaType == MimeEventStream || mediaType == MimeRaw {
			return mediaType
		}
	}
	return MimeHal
}
