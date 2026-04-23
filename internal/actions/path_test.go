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
	maxAssetsParamLength = 2
)

func TestStrictReceivePathsQueryAmount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		amount  string
		wantErr bool
	}{
		{name: "valid", amount: "10.0", wantErr: false},
		{name: "empty", amount: "", wantErr: true},
		{name: "not a number", amount: "abc", wantErr: true},
		{name: "malformed decimal", amount: "1.2.3", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := StrictReceivePathsQuery{DestinationAmount: tc.amount}
			var err error
			assert.NotPanics(t, func() { _, err = q.Amount() })
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestStrictReceivePathsQueryDestinationAsset(t *testing.T) {
	for _, tc := range []struct {
		name      string
		assetType string
		code      string
		issuer    string
		wantErr   bool
	}{
		{name: "native", assetType: "native", wantErr: false},
		{name: "credit", assetType: "credit_alphanum4", code: "USD", issuer: dummyIssuer, wantErr: false},
		{name: "invalid type", assetType: "garbage", wantErr: true},
		{name: "bad issuer", assetType: "credit_alphanum4", code: "USD", issuer: "not-an-address", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := StrictReceivePathsQuery{
				DestinationAssetType:   tc.assetType,
				DestinationAssetCode:   tc.code,
				DestinationAssetIssuer: tc.issuer,
			}
			var err error
			assert.NotPanics(t, func() { _, err = q.DestinationAsset() })
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFindFixedPathsQueryAmount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		amount  string
		wantErr bool
	}{
		{name: "valid", amount: "10.0", wantErr: false},
		{name: "empty", amount: "", wantErr: true},
		{name: "not a number", amount: "abc", wantErr: true},
		{name: "malformed decimal", amount: "1.2.3", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := FindFixedPathsQuery{SourceAmount: tc.amount}
			var err error
			assert.NotPanics(t, func() { _, err = q.Amount() })
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFindFixedPathsQuerySourceAsset(t *testing.T) {
	for _, tc := range []struct {
		name      string
		assetType string
		code      string
		issuer    string
		wantErr   bool
	}{
		{name: "native", assetType: "native", wantErr: false},
		{name: "credit", assetType: "credit_alphanum4", code: "USD", issuer: dummyIssuer, wantErr: false},
		{name: "invalid type", assetType: "garbage", wantErr: true},
		{name: "bad issuer", assetType: "credit_alphanum4", code: "USD", issuer: "not-an-address", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := FindFixedPathsQuery{
				SourceAssetType:   tc.assetType,
				SourceAssetCode:   tc.code,
				SourceAssetIssuer: tc.issuer,
			}
			var err error
			assert.NotPanics(t, func() { _, err = q.SourceAsset() })
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

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

	_, _, err := assetsForAddressWithLimit(r.WithContext(ctx), address, maxAssetsParamLength)
	assert.EqualError(t, err, "cannot be called outside of a transaction")

	assert.NoError(t, q.Begin(ctx))
	defer q.Rollback()

	_, _, err = assetsForAddressWithLimit(r.WithContext(ctx), address, maxAssetsParamLength)
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

	var err error
	_, _, err = assetsForAddressWithLimit(r.WithContext(ctx), address, maxAssetsParamLength)
	assert.NoError(t, err)

	// With limit 1: 2 assets > 1, should fail
	_, _, err = assetsForAddressWithLimit(r.WithContext(ctx), address, 1)
	assert.EqualError(t, err, "account has too many trustlines to use this endpoint (number of trustlines plus native XLM exceeds limit of 1)")
}
