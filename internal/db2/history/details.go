package history

import "bytes"

// jsonNullEscape is how encoding/json renders a NUL character (U+0000): the six
// ASCII bytes backslash, 'u', '0', '0', '0', '0'. Postgres jsonb cannot store a
// NUL, because it decodes escapes into text and text cannot hold one (a plain
// json column would accept it), so this escape must be removed before a
// marshaled document is written to a jsonb column.
var jsonNullEscape = []byte{0x5c, 'u', '0', '0', '0', '0'}

// sanitizeJSONBDetails makes a marshaled JSON document safe to store in a jsonb
// column by removing any NUL characters it contains.
//
// Strings derived from ledger data are not guaranteed to be free of bytes that
// jsonb cannot represent: asset codes are opaque 4/12-byte arrays and Soroban
// ScVal strings are arbitrary bytes. A NUL in particular would cause the insert
// to fail.
//
// encoding/json renders a real NUL as the jsonNullEscape sequence, but those
// same six bytes can also occur as the tail of an escaped backslash followed by
// the literal text "u0000" (for example a string whose value is the six literal
// characters marshals with a doubled leading backslash). Only an escape
// introduced by an unescaped backslash represents an actual NUL, so we walk the
// document consuming each escape as a unit and drop only genuine NUL escapes; an
// escaped-backslash pair is copied verbatim. This never alters a legitimate
// value and always leaves valid JSON.
func sanitizeJSONBDetails(details []byte) []byte {
	if !bytes.Contains(details, jsonNullEscape) {
		return details
	}
	out := make([]byte, 0, len(details))
	for i := 0; i < len(details); {
		if details[i] != '\\' {
			out = append(out, details[i])
			i++
			continue
		}
		// An escape sequence starts here. If it is a genuine NUL escape, drop it.
		if bytes.HasPrefix(details[i:], jsonNullEscape) {
			i += len(jsonNullEscape)
			continue
		}
		// Otherwise copy this backslash together with the character it escapes,
		// so an escaped-backslash pair is consumed as a unit and its second
		// backslash cannot be mistaken for the start of a NUL escape.
		out = append(out, details[i])
		i++
		if i < len(details) {
			out = append(out, details[i])
			i++
		}
	}
	return out
}
