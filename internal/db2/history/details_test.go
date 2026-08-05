package history

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/guregu/null"

	"github.com/stellar/go-stellar-sdk/toid"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/db2"
	"github.com/stellar/stellar-horizon/internal/test"
)

const nulByteTestIssuer = "GA7QYNF7SOWQ3GLR2BGMZEHXAVIRZA4KVWLTJJFC7MGXUA74P7UJVSGZ"

// literalNulEscape is the six literal characters backslash, 'u', '0', '0', '0',
// '0' — the text that spells a NUL escape, but is NOT a NUL. It is built from
// bytes to avoid any real escape sequence in the source.
var literalNulEscape = string([]byte{0x5c, 'u', '0', '0', '0', '0'})

// nulByteTestDetails returns marshaled details whose asset code contains an
// interior NUL — a value that can occur in ledger-derived data. encoding/json
// renders the NUL as jsonNullEscape, which a jsonb column cannot store, so this
// exercises the sanitizeJSONBDetails path. sanitizedAsset is what the asset must
// read as once the NUL has been stripped.
func nulByteTestDetails(t *testing.T) (marshaled []byte, sanitizedAsset string) {
	t.Helper()
	poisonedCode := string([]byte{'A', 0x00, 'B'}) // 'A' NUL 'B'
	details, err := json.Marshal(map[string]string{
		"from":  "asset",
		"asset": poisonedCode + ":" + nulByteTestIssuer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(details, jsonNullEscape) {
		t.Fatalf("expected marshaled details to contain the NUL escape, got %s", details)
	}
	return details, "AB:" + nulByteTestIssuer
}

func TestSanitizeJSONBDetails(t *testing.T) {
	mustMarshal := func(v map[string]string) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}

	poisoned, wantAsset := nulByteTestDetails(t)

	got := sanitizeJSONBDetails(poisoned)

	if bytes.Contains(got, jsonNullEscape) {
		t.Errorf("sanitized details still contain the NUL escape: %s", got)
	}
	if strings.ContainsRune(string(got), rune(0)) {
		t.Errorf("sanitized details still contain a raw NUL: %q", string(got))
	}
	if !json.Valid(got) {
		t.Errorf("sanitized details are not valid JSON: %s", got)
	}
	var out map[string]string
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("cannot unmarshal sanitized details: %v", err)
	}
	if out["asset"] != wantAsset {
		t.Errorf("asset = %q, want %q", out["asset"], wantAsset)
	}

	// A document with no NUL escape must be returned unchanged.
	clean := []byte(`{"asset": "AB:` + nulByteTestIssuer + `", "from": "asset"}`)
	if got := sanitizeJSONBDetails(clean); !bytes.Equal(got, clean) {
		t.Errorf("clean input was modified: got %s want %s", got, clean)
	}

	// A value that legitimately contains the literal escape text (a backslash
	// followed by u0000, not a NUL) marshals with a doubled leading backslash. It
	// must be preserved, not mistaken for a NUL escape and truncated into invalid
	// JSON.
	withLiteral := mustMarshal(map[string]string{"asset": literalNulEscape})
	sanitized := sanitizeJSONBDetails(withLiteral)
	if !json.Valid(sanitized) {
		t.Errorf("literal-escape input produced invalid JSON: %s", sanitized)
	}
	var lit map[string]string
	if err := json.Unmarshal(sanitized, &lit); err != nil {
		t.Fatalf("cannot unmarshal sanitized literal-escape details: %v", err)
	}
	if lit["asset"] != literalNulEscape {
		t.Errorf("literal escape value corrupted: got %q want %q", lit["asset"], literalNulEscape)
	}

	// A value with both a real NUL and the literal escape text: only the NUL is
	// removed, the literal text survives.
	mixed := mustMarshal(map[string]string{
		"asset": string([]byte{'A', 0x00}) + literalNulEscape,
	})
	sanitizedMixed := sanitizeJSONBDetails(mixed)
	if !json.Valid(sanitizedMixed) {
		t.Errorf("mixed input produced invalid JSON: %s", sanitizedMixed)
	}
	var mix map[string]string
	if err := json.Unmarshal(sanitizedMixed, &mix); err != nil {
		t.Fatalf("cannot unmarshal sanitized mixed details: %v", err)
	}
	if want := "A" + literalNulEscape; mix["asset"] != want {
		t.Errorf("mixed value: got %q want %q", mix["asset"], want)
	}
}

// TestAddOperationSanitizesNulByteInDetails checks that a NUL-bearing details
// document inserts into the real jsonb column instead of failing.
func TestAddOperationSanitizesNulByteInDetails(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &Q{tt.HorizonSession()}

	tt.Require.NoError(q.Begin(tt.Ctx))

	details, wantAsset := nulByteTestDetails(t)

	builder := q.NewOperationBatchInsertBuilder()
	sequence := int32(56)
	opID := toid.New(sequence, 1, 1).ToInt64()
	sourceAccount := "GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY"

	// Without sanitizeJSONBDetails this insert fails because jsonb cannot store a NUL.
	tt.Require.NoError(builder.Add(
		opID,
		toid.New(sequence, 1, 0).ToInt64(),
		1,
		xdr.OperationTypeInvokeHostFunction,
		details,
		sourceAccount,
		null.String{},
		false,
	))
	tt.Require.NoError(builder.Exec(tt.Ctx, q))
	tt.Require.NoError(q.Commit())

	var stored string
	tt.Require.NoError(q.GetRaw(tt.Ctx, &stored,
		"SELECT details FROM history_operations WHERE id = $1", opID))
	tt.Assert.False(strings.ContainsRune(stored, rune(0)), "stored details must not contain a NUL")
	tt.Assert.Contains(stored, wantAsset)
}

// TestAddEffectSanitizesNulByteInDetails guards the second jsonb sink,
// history_effects.details.
func TestAddEffectSanitizesNulByteInDetails(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &Q{tt.HorizonSession()}

	tt.Require.NoError(q.Begin(tt.Ctx))

	details, wantAsset := nulByteTestDetails(t)

	address := "GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY"
	accountLoader := NewAccountLoader(ConcurrentInserts)
	builder := q.NewEffectBatchInsertBuilder()
	sequence := int32(56)

	// Without sanitizeJSONBDetails this insert fails because jsonb cannot store a NUL.
	tt.Require.NoError(builder.Add(
		accountLoader.GetFuture(address),
		null.String{},
		toid.New(sequence, 1, 1).ToInt64(),
		1,
		EffectType(3),
		details,
	))
	tt.Require.NoError(accountLoader.Exec(tt.Ctx, q))
	tt.Require.NoError(builder.Exec(tt.Ctx, q))
	tt.Require.NoError(q.Commit())

	effects, err := q.Effects(tt.Ctx, db2.PageQuery{Cursor: "0-0", Order: "asc", Limit: 200}, 0)
	tt.Require.NoError(err)
	tt.Require.Len(effects, 1)
	stored := effects[0].DetailsString.String
	tt.Assert.False(strings.ContainsRune(stored, rune(0)), "stored details must not contain a NUL")
	tt.Assert.Contains(stored, wantAsset)
}
