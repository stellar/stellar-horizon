package history

import (
	"bytes"
	"encoding/json"
	"strings"
)

// jsonNullEscape is how encoding/json renders a NUL character (U+0000): the six
// ASCII bytes backslash, 'u', '0', '0', '0', '0'. Postgres jsonb cannot store a
// NUL, because it decodes escapes into text and text cannot hold one (a plain
// json column would accept it), so a marshaled document containing one cannot be
// written to a jsonb column as-is. It is used only as a cheap pre-check.
var jsonNullEscape = []byte{0x5c, 'u', '0', '0', '0', '0'}

// sanitizeJSONBDetails makes a marshaled JSON document safe to store in a jsonb
// column by removing NUL characters from every string it contains.
//
// Strings derived from ledger data are not guaranteed to be free of bytes that
// jsonb cannot represent: asset codes are opaque 4/12-byte arrays and Soroban
// ScVal strings are arbitrary bytes. A NUL would otherwise cause the insert to
// fail.
//
// The common case — no NUL — returns the input untouched. Otherwise the document
// is decoded, NUL is stripped from its string values (and any object keys), and
// it is re-encoded, leaving all JSON parsing and escaping to encoding/json
// rather than editing the serialized bytes by hand. UseNumber keeps numeric
// values exact across the round trip.
func sanitizeJSONBDetails(details []byte) []byte {
	if !bytes.Contains(details, jsonNullEscape) {
		return details
	}

	decoder := json.NewDecoder(bytes.NewReader(details))
	decoder.UseNumber()
	var decoded interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return details
	}

	sanitized, err := json.Marshal(stripNULFromJSON(decoded))
	if err != nil {
		return details
	}
	return sanitized
}

// stripNULFromJSON removes NUL characters from every string within a value
// decoded from JSON. Such a value only ever holds string, json.Number, bool,
// nil, []interface{} and map[string]interface{} — never a struct — so recursion
// over these cases reaches every string.
func stripNULFromJSON(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return stripNUL(v)
	case []interface{}:
		for i := range v {
			v[i] = stripNULFromJSON(v[i])
		}
		return v
	case map[string]interface{}:
		sanitized := make(map[string]interface{}, len(v))
		for key, val := range v {
			sanitized[stripNUL(key)] = stripNULFromJSON(val)
		}
		return sanitized
	default:
		return v
	}
}

// stripNUL removes every NUL character (U+0000) from s.
func stripNUL(s string) string {
	if !strings.ContainsRune(s, 0) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == 0 {
			return -1
		}
		return r
	}, s)
}
