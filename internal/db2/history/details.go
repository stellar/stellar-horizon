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
// encoding/json always renders a NUL as the jsonNullEscape sequence and always
// emits valid UTF-8, so stripping that escape from the already-marshaled bytes
// is sufficient to guarantee the document is jsonb-storable. Removing the
// self-contained six-byte escape from a JSON string literal cannot change JSON
// validity or affect any other value.
func sanitizeJSONBDetails(details []byte) []byte {
	if !bytes.Contains(details, jsonNullEscape) {
		return details
	}
	return bytes.ReplaceAll(details, jsonNullEscape, nil)
}
