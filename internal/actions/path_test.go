package actions

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	horizonContext "github.com/stellar/stellar-horizon/internal/context"
	"github.com/stellar/stellar-horizon/internal/db2/history"
	"github.com/stellar/stellar-horizon/internal/test"
	"github.com/stretchr/testify/assert"
)

var (
	address              = "GCATOZ7YJV2FANQQLX47TIV6P7VMPJCEEJGQGR6X7TONPKBN3UCLKEIS"
	dummyIssuer          = "GBRPYHIL2CI3FNQ4BXLFMNDLFJUNPU2HY3ZMFSHONUCEOASW7QC7OX2H"
	MaxAssetsParamLength = 2
)

func TestAssetsForAddressRequiresTransaction(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &history.Q{tt.HorizonSession()}

	r := &http.Request{}
	ctx := context.WithValue(
		r.Context(),
		&horizonContext.SessionContextKey,
		q,
	)

	_, _, err := assetsForAddressWithLimit(r.WithContext(ctx), address, MaxAssetsParamLength)
	assert.EqualError(t, err, "cannot be called outside of a transaction")

	assert.NoError(t, q.Begin(ctx))
	defer q.Rollback()

	_, _, err = assetsForAddressWithLimit(r.WithContext(ctx), address, MaxAssetsParamLength)
	assert.EqualError(t, err, "should only be called in a repeatable read transaction")
}

func TestAssetLimits(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)
	q := &history.Q{tt.HorizonSession()}

	// Insert account + 1 trustline so the function returns 2 assets (trustline + native XLM).
	assert.NoError(t, q.UpsertAccounts(tt.Ctx, []history.AccountEntry{
		{AccountID: address, Balance: 20000, LastModifiedLedger: 1},
	}))
	assert.NoError(t, q.UpsertTrustLines(tt.Ctx, []history.TrustLine{
		{
			AccountID: address, AssetType: 2, AssetIssuer: dummyIssuer,
			AssetCode: "EUR", LedgerKey: "k1", LastModifiedLedger: 1,
		},
	}))

	r := &http.Request{}
	ctx := context.WithValue(
		r.Context(),
		&horizonContext.SessionContextKey,
		q,
	)

	assert.NoError(t, q.BeginTx(ctx, &sql.TxOptions{
		ReadOnly:  true,
		Isolation: sql.LevelRepeatableRead,
	}))
	defer q.Rollback()

	// account, err := q.AccountByAddress(ctx, address)
	// assert.NoError(t, err)
	var err error
	_, _, err = assetsForAddressWithLimit(r.WithContext(ctx), address, MaxAssetsParamLength)
	assert.NoError(t, err)

	// With limit 1: 2 assets > 1, should fail
	_, _, err = assetsForAddressWithLimit(r.WithContext(ctx), address, 1)
	assert.EqualError(t, err, "number of assets exceeds maximum length of 1")
}
