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
}

// TestAddOperationSanitizesNulByteInDetails checks that a NUL-bearing details
// document inserts into the real jsonb column instead of failing.
func TestAddOperationSanitizesNulByteInDetails(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &Q{tt.HorizonSession()}

	tt.Assert.NoError(q.Begin(tt.Ctx))

	details, wantAsset := nulByteTestDetails(t)

	builder := q.NewOperationBatchInsertBuilder()
	sequence := int32(56)
	opID := toid.New(sequence, 1, 1).ToInt64()
	sourceAccount := "GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY"

	// Without sanitizeJSONBDetails this insert fails because jsonb cannot store a NUL.
	tt.Assert.NoError(builder.Add(
		opID,
		toid.New(sequence, 1, 0).ToInt64(),
		1,
		xdr.OperationTypeInvokeHostFunction,
		details,
		sourceAccount,
		null.String{},
		false,
	))
	tt.Assert.NoError(builder.Exec(tt.Ctx, q))
	tt.Assert.NoError(q.Commit())

	var stored string
	tt.Assert.NoError(q.GetRaw(tt.Ctx, &stored,
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

	tt.Assert.NoError(q.Begin(tt.Ctx))

	details, wantAsset := nulByteTestDetails(t)

	address := "GAQAA5L65LSYH7CQ3VTJ7F3HHLGCL3DSLAR2Y47263D56MNNGHSQSTVY"
	accountLoader := NewAccountLoader(ConcurrentInserts)
	builder := q.NewEffectBatchInsertBuilder()
	sequence := int32(56)

	// Without sanitizeJSONBDetails this insert fails because jsonb cannot store a NUL.
	tt.Assert.NoError(builder.Add(
		accountLoader.GetFuture(address),
		null.String{},
		toid.New(sequence, 1, 1).ToInt64(),
		1,
		EffectType(3),
		details,
	))
	tt.Assert.NoError(accountLoader.Exec(tt.Ctx, q))
	tt.Assert.NoError(builder.Exec(tt.Ctx, q))
	tt.Assert.NoError(q.Commit())

	effects, err := q.Effects(tt.Ctx, db2.PageQuery{Cursor: "0-0", Order: "asc", Limit: 200}, 0)
	tt.Assert.NoError(err)
	tt.Assert.Len(effects, 1)
	stored := effects[0].DetailsString.String
	tt.Assert.False(strings.ContainsRune(stored, rune(0)), "stored details must not contain a NUL")
	tt.Assert.Contains(stored, wantAsset)
}
