package filters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/ingest"
	"github.com/stellar/go-stellar-sdk/ingest/sac"
	"github.com/stellar/go-stellar-sdk/xdr"
	"github.com/stellar/stellar-horizon/internal/db2/history"
)

func TestAssetFilterAllowsOnMatch(t *testing.T) {
	tt := assert.New(t)
	ctx := context.Background()

	filterConfig := &history.AssetFilterConfig{
		Whitelist:    []string{"USDC:GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"},
		Enabled:      true,
		LastModified: 1,
	}
	filter := NewAssetFilter(testNetworkPassphrase)
	err := filter.RefreshAssetFilter(filterConfig)
	tt.NoError(err)

	isEnabled, result, err := filter.FilterTransaction(ctx, getAssetTestV1Tx(t, "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"))
	tt.NoError(err)
	tt.Equal(isEnabled, true)
	tt.Equal(result, true)

	isEnabled, result, err = filter.FilterTransaction(ctx, getAssetTestV0Tx(t, "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"))
	tt.NoError(err)
	tt.Equal(isEnabled, true)
	tt.Equal(result, true)
}

func TestAssetFilterAllowsWhenEmptyWhitelist(t *testing.T) {
	tt := assert.New(t)
	ctx := context.Background()

	filterConfig := &history.AssetFilterConfig{
		Whitelist:    []string{},
		Enabled:      true,
		LastModified: 1,
	}
	filter := NewAssetFilter(testNetworkPassphrase)
	err := filter.RefreshAssetFilter(filterConfig)
	tt.NoError(err)

	isEnabled, result, err := filter.FilterTransaction(ctx, getAssetTestV1Tx(t, "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"))
	tt.NoError(err)
	tt.Equal(isEnabled, false)
	tt.Equal(result, true)

	isEnabled, result, err = filter.FilterTransaction(ctx, getAssetTestV0Tx(t, "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"))
	tt.NoError(err)
	tt.Equal(isEnabled, false)
	tt.Equal(result, true)
}

func TestAssetFilterAllowsWhenDisabled(t *testing.T) {
	tt := assert.New(t)
	ctx := context.Background()

	filterConfig := &history.AssetFilterConfig{
		Whitelist:    []string{"USDX:GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"},
		Enabled:      false,
		LastModified: 1,
	}
	filter := NewAssetFilter(testNetworkPassphrase)
	err := filter.RefreshAssetFilter(filterConfig)
	tt.NoError(err)

	isEnabled, result, err := filter.FilterTransaction(ctx, getAssetTestV1Tx(t, "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"))
	tt.NoError(err)
	// there was no match on filter rules, but since filter was disabled also, it should allow all
	tt.Equal(isEnabled, false)
	tt.Equal(result, true)
}

func TestAssetFilterDoesNotAllowV1WhenNoMatch(t *testing.T) {
	tt := assert.New(t)
	ctx := context.Background()

	filterConfig := &history.AssetFilterConfig{
		Whitelist:    []string{"USDX:GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"},
		Enabled:      true,
		LastModified: 1,
	}

	filter := NewAssetFilter(testNetworkPassphrase)
	err := filter.RefreshAssetFilter(filterConfig)
	tt.NoError(err)

	isEnabled, result, err := filter.FilterTransaction(ctx, getAssetTestV1Tx(t, "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"))
	tt.NoError(err)
	tt.Equal(isEnabled, true)
	tt.Equal(result, false)

	isEnabled, result, err = filter.FilterTransaction(ctx, getAssetTestV0Tx(t, "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"))
	tt.NoError(err)
	tt.Equal(isEnabled, true)
	tt.Equal(result, false)
}

func getAssetTestV1Tx(t *testing.T, issuer string) ingest.LedgerTransaction {
	var xdrAssetCode [12]byte
	var xdrIssuer xdr.AccountId
	copy(xdrAssetCode[:], "USDC")
	require.NoError(t, xdrIssuer.SetAddress(issuer))

	return ingest.LedgerTransaction{
		Result: xdr.TransactionResultPair{
			Result: xdr.TransactionResult{
				Result: xdr.TransactionResultResult{
					Code: xdr.TransactionResultCodeTxSuccess,
				},
			},
		},
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTx,
			V1: &xdr.TransactionV1Envelope{
				Tx: xdr.Transaction{
					Operations: []xdr.Operation{
						{Body: xdr.OperationBody{
							Type: xdr.OperationTypePayment,
							PaymentOp: &xdr.PaymentOp{
								Destination: xdr.MustMuxedAddress("GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"),
								Asset: xdr.Asset{
									Type: xdr.AssetTypeAssetTypeCreditAlphanum12,
									AlphaNum12: &xdr.AlphaNum12{
										AssetCode: xdrAssetCode,
										Issuer:    xdrIssuer,
									},
								},
								Amount: 100,
							},
						}},
					},
				},
			},
		},
	}
}

func getAssetTestV0Tx(t *testing.T, issuer string) ingest.LedgerTransaction {
	var xdrAssetCode [12]byte
	var xdrIssuer xdr.AccountId
	copy(xdrAssetCode[:], "USDC")
	require.NoError(t, xdrIssuer.SetAddress(issuer))

	return ingest.LedgerTransaction{
		Result: xdr.TransactionResultPair{
			Result: xdr.TransactionResult{
				Result: xdr.TransactionResultResult{
					Code: xdr.TransactionResultCodeTxSuccess,
				},
			},
		},
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTxV0,
			V0: &xdr.TransactionV0Envelope{
				Tx: xdr.TransactionV0{
					Operations: []xdr.Operation{
						{Body: xdr.OperationBody{
							Type: xdr.OperationTypePayment,
							PaymentOp: &xdr.PaymentOp{
								Destination: xdr.MustMuxedAddress("GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"),
								Asset: xdr.Asset{
									Type: xdr.AssetTypeAssetTypeCreditAlphanum12,
									AlphaNum12: &xdr.AlphaNum12{
										AssetCode: xdrAssetCode,
										Issuer:    xdrIssuer,
									},
								},
								Amount: 100,
							},
						}},
					},
				},
			},
		},
	}
}

func wrapInFeeBump(inner ingest.LedgerTransaction) ingest.LedgerTransaction {
	return ingest.LedgerTransaction{
		Result: inner.Result,
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTxFeeBump,
			FeeBump: &xdr.FeeBumpTransactionEnvelope{
				Tx: xdr.FeeBumpTransaction{
					FeeSource: xdr.MustMuxedAddress("GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"),
					Fee:       2000,
					InnerTx: xdr.FeeBumpTransactionInnerTx{
						Type: xdr.EnvelopeTypeEnvelopeTypeTx,
						V1:   inner.Envelope.V1,
					},
				},
			},
		},
	}
}

func TestAssetFilterMatchesFeeBumpInnerOperations(t *testing.T) {
	tt := assert.New(t)
	ctx := context.Background()
	issuer := "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"

	filter := NewAssetFilter(testNetworkPassphrase)
	tt.NoError(filter.RefreshAssetFilter(&history.AssetFilterConfig{
		Whitelist:    []string{"USDC:" + issuer},
		Enabled:      true,
		LastModified: 1,
	}))

	isEnabled, result, err := filter.FilterTransaction(ctx, wrapInFeeBump(getAssetTestV1Tx(t, issuer)))
	tt.NoError(err)
	tt.True(isEnabled)
	tt.True(result)
}

func TestAssetFilterDoesNotAllowFeeBumpWhenNoMatch(t *testing.T) {
	tt := assert.New(t)
	ctx := context.Background()
	issuer := "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"

	filter := NewAssetFilter(testNetworkPassphrase)
	tt.NoError(filter.RefreshAssetFilter(&history.AssetFilterConfig{
		Whitelist:    []string{"USDX:" + issuer},
		Enabled:      true,
		LastModified: 1,
	}))

	isEnabled, result, err := filter.FilterTransaction(ctx, wrapInFeeBump(getAssetTestV1Tx(t, issuer)))
	tt.NoError(err)
	tt.True(isEnabled)
	tt.False(result)
}

func TestAssetFilterMatchesPathPaymentIntermediateAsset(t *testing.T) {
	tt := assert.New(t)
	ctx := context.Background()

	issuer := "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	other1 := xdr.MustNewCreditAsset("EURC", issuer)
	other2 := xdr.MustNewCreditAsset("GBPC", issuer)

	filter := NewAssetFilter(testNetworkPassphrase)
	tt.NoError(filter.RefreshAssetFilter(&history.AssetFilterConfig{
		Whitelist:    []string{whitelisted.StringCanonical()},
		Enabled:      true,
		LastModified: 1,
	}))

	for _, op := range []xdr.Operation{
		{Body: xdr.OperationBody{
			Type: xdr.OperationTypePathPaymentStrictSend,
			PathPaymentStrictSendOp: &xdr.PathPaymentStrictSendOp{
				SendAsset:   other1,
				SendAmount:  100,
				Destination: xdr.MustMuxedAddress(issuer),
				DestAsset:   other2,
				DestMin:     1,
				Path:        []xdr.Asset{whitelisted},
			},
		}},
		{Body: xdr.OperationBody{
			Type: xdr.OperationTypePathPaymentStrictReceive,
			PathPaymentStrictReceiveOp: &xdr.PathPaymentStrictReceiveOp{
				SendAsset:   other1,
				SendMax:     100,
				Destination: xdr.MustMuxedAddress(issuer),
				DestAsset:   other2,
				DestAmount:  1,
				Path:        []xdr.Asset{whitelisted},
			},
		}},
	} {
		tx := ingest.LedgerTransaction{
			Envelope: xdr.TransactionEnvelope{
				Type: xdr.EnvelopeTypeEnvelopeTypeTx,
				V1:   &xdr.TransactionV1Envelope{Tx: xdr.Transaction{Operations: []xdr.Operation{op}}},
			},
		}
		isEnabled, include, err := filter.FilterTransaction(ctx, tx)
		tt.NoError(err)
		tt.True(isEnabled)
		tt.True(include, "operation type %s", op.Body.Type)
	}
}

const (
	testNetworkPassphrase = "Test SDF Network ; September 2015"
	testIssuer            = "GD6WNNTW664WH7FXC5RUMUTF7P5QSURC2IT36VOQEEGFZ4UWUEQGECAL"
	testHolder            = "GAHK7EEG2WWHVKDNT4CEQFZGKF2LGDSW2IVM4S5DP42RBW3K6BTODB4A"
)

func newTestAssetFilter(t *testing.T, whitelist ...xdr.Asset) AssetFilter {
	canonical := make([]string, 0, len(whitelist))
	for _, asset := range whitelist {
		canonical = append(canonical, asset.StringCanonical())
	}
	filter := NewAssetFilter(testNetworkPassphrase)
	require.NoError(t, filter.RefreshAssetFilter(&history.AssetFilterConfig{
		Whitelist:    canonical,
		Enabled:      true,
		LastModified: 1,
	}))
	return filter
}

func successfulTxWithMetaV3(source string, ops []xdr.Operation, changes xdr.LedgerEntryChanges) ingest.LedgerTransaction {
	return ingest.LedgerTransaction{
		Result: xdr.TransactionResultPair{
			Result: xdr.TransactionResult{
				Result: xdr.TransactionResultResult{Code: xdr.TransactionResultCodeTxSuccess},
			},
		},
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTx,
			V1: &xdr.TransactionV1Envelope{
				Tx: xdr.Transaction{
					SourceAccount: xdr.MustMuxedAddress(source),
					Operations:    ops,
				},
			},
		},
		UnsafeMeta: xdr.TransactionMeta{
			V: 3,
			V3: &xdr.TransactionMetaV3{
				Operations: []xdr.OperationMeta{{Changes: changes}},
			},
		},
	}
}

func stateAndRemoved(entry xdr.LedgerEntry) xdr.LedgerEntryChanges {
	key, err := entry.LedgerKey()
	if err != nil {
		panic(err)
	}
	return xdr.LedgerEntryChanges{
		{Type: xdr.LedgerEntryChangeTypeLedgerEntryState, State: &entry},
		{Type: xdr.LedgerEntryChangeTypeLedgerEntryRemoved, Removed: &key},
	}
}

func stateAndUpdated(before, after xdr.LedgerEntry) xdr.LedgerEntryChanges {
	return xdr.LedgerEntryChanges{
		{Type: xdr.LedgerEntryChangeTypeLedgerEntryState, State: &before},
		{Type: xdr.LedgerEntryChangeTypeLedgerEntryUpdated, Updated: &after},
	}
}

func TestAssetFilterMatchesSetTrustLineFlags(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	tx := successfulTxWithMetaV3(issuer, []xdr.Operation{{Body: xdr.OperationBody{
		Type: xdr.OperationTypeSetTrustLineFlags,
		SetTrustLineFlagsOp: &xdr.SetTrustLineFlagsOp{
			Trustor:  xdr.MustAddress(issuer),
			Asset:    whitelisted,
			SetFlags: xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag),
		},
	}}}, nil)

	isEnabled, include, err := filter.FilterTransaction(context.Background(), tx)
	tt.NoError(err)
	tt.True(isEnabled)
	tt.True(include)
}

func TestAssetFilterMatchesAllowTrustUsingIssuerAsSource(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	other := testHolder
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	code, codeErr := xdr.NewAssetCodeFromString("USDC")
	tt.NoError(codeErr)
	allowTrust := func(opSource *xdr.MuxedAccount) xdr.Operation {
		return xdr.Operation{
			SourceAccount: opSource,
			Body: xdr.OperationBody{
				Type: xdr.OperationTypeAllowTrust,
				AllowTrustOp: &xdr.AllowTrustOp{
					Trustor:   xdr.MustAddress(other),
					Asset:     code,
					Authorize: xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag),
				},
			},
		}
	}
	issuerMuxed := xdr.MustMuxedAddress(issuer)

	for name, tx := range map[string]ingest.LedgerTransaction{
		"tx source is issuer": successfulTxWithMetaV3(issuer, []xdr.Operation{allowTrust(nil)}, nil),
		"op source is issuer": successfulTxWithMetaV3(other, []xdr.Operation{allowTrust(&issuerMuxed)}, nil),
	} {
		isEnabled, include, err := filter.FilterTransaction(context.Background(), tx)
		tt.NoError(err, name)
		tt.True(isEnabled, name)
		tt.True(include, name)
	}

	notIssuer := successfulTxWithMetaV3(other, []xdr.Operation{allowTrust(nil)}, nil)
	_, include, err := filter.FilterTransaction(context.Background(), notIssuer)
	tt.NoError(err)
	tt.False(include)
}

func TestAssetFilterMatchesRevokeSponsorshipOfTrustline(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	other := testHolder
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	key := xdr.LedgerKey{
		Type: xdr.LedgerEntryTypeTrustline,
		TrustLine: &xdr.LedgerKeyTrustLine{
			AccountId: xdr.MustAddress(other),
			Asset:     whitelisted.ToTrustLineAsset(),
		},
	}
	tx := successfulTxWithMetaV3(issuer, []xdr.Operation{{Body: xdr.OperationBody{
		Type: xdr.OperationTypeRevokeSponsorship,
		RevokeSponsorshipOp: &xdr.RevokeSponsorshipOp{
			Type:      xdr.RevokeSponsorshipTypeRevokeSponsorshipLedgerEntry,
			LedgerKey: &key,
		},
	}}}, nil)

	isEnabled, include, err := filter.FilterTransaction(context.Background(), tx)
	tt.NoError(err)
	tt.True(isEnabled)
	tt.True(include)
}

func claimableBalanceEntry(asset xdr.Asset) xdr.LedgerEntry {
	return xdr.LedgerEntry{
		Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeClaimableBalance,
			ClaimableBalance: &xdr.ClaimableBalanceEntry{
				BalanceId: xdr.ClaimableBalanceId{
					Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
					V0:   &xdr.Hash{1},
				},
				Asset:  asset,
				Amount: 100,
			},
		},
	}
}

func TestAssetFilterMatchesClaimableBalanceClaim(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	other := xdr.MustNewCreditAsset("EURC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	claim := []xdr.Operation{{Body: xdr.OperationBody{
		Type: xdr.OperationTypeClaimClaimableBalance,
		ClaimClaimableBalanceOp: &xdr.ClaimClaimableBalanceOp{
			BalanceId: xdr.ClaimableBalanceId{
				Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
				V0:   &xdr.Hash{1},
			},
		},
	}}}

	_, include, err := filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(issuer, claim, stateAndRemoved(claimableBalanceEntry(whitelisted))))
	tt.NoError(err)
	tt.True(include)

	_, include, err = filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(issuer, claim, stateAndRemoved(claimableBalanceEntry(other))))
	tt.NoError(err)
	tt.False(include)
}

func TestAssetFilterMatchesClaimableBalanceClawback(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	clawback := []xdr.Operation{{Body: xdr.OperationBody{
		Type: xdr.OperationTypeClawbackClaimableBalance,
		ClawbackClaimableBalanceOp: &xdr.ClawbackClaimableBalanceOp{
			BalanceId: xdr.ClaimableBalanceId{
				Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
				V0:   &xdr.Hash{1},
			},
		},
	}}}

	_, include, err := filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(issuer, clawback, stateAndRemoved(claimableBalanceEntry(whitelisted))))
	tt.NoError(err)
	tt.True(include)
}

func liquidityPoolEntry(assetA, assetB xdr.Asset, reserveA xdr.Int64) xdr.LedgerEntry {
	return xdr.LedgerEntry{
		Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeLiquidityPool,
			LiquidityPool: &xdr.LiquidityPoolEntry{
				LiquidityPoolId: xdr.PoolId{2},
				Body: xdr.LiquidityPoolEntryBody{
					Type: xdr.LiquidityPoolTypeLiquidityPoolConstantProduct,
					ConstantProduct: &xdr.LiquidityPoolEntryConstantProduct{
						Params: xdr.LiquidityPoolConstantProductParameters{
							AssetA: assetA,
							AssetB: assetB,
							Fee:    30,
						},
						ReserveA:        reserveA,
						ReserveB:        reserveA,
						TotalPoolShares: reserveA,
					},
				},
			},
		},
	}
}

func TestAssetFilterMatchesLiquidityPoolDepositAndWithdraw(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	other := xdr.MustNewCreditAsset("EURC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	poolChanges := stateAndUpdated(liquidityPoolEntry(other, whitelisted, 100), liquidityPoolEntry(other, whitelisted, 200))
	native := xdr.MustNewNativeAsset()
	unrelatedPoolChanges := stateAndUpdated(liquidityPoolEntry(other, native, 100), liquidityPoolEntry(other, native, 200))

	for name, op := range map[string]xdr.Operation{
		"deposit": {Body: xdr.OperationBody{
			Type: xdr.OperationTypeLiquidityPoolDeposit,
			LiquidityPoolDepositOp: &xdr.LiquidityPoolDepositOp{
				LiquidityPoolId: xdr.PoolId{2},
				MaxAmountA:      100,
				MaxAmountB:      100,
				MinPrice:        xdr.Price{N: 1, D: 1},
				MaxPrice:        xdr.Price{N: 1, D: 1},
			},
		}},
		"withdraw": {Body: xdr.OperationBody{
			Type: xdr.OperationTypeLiquidityPoolWithdraw,
			LiquidityPoolWithdrawOp: &xdr.LiquidityPoolWithdrawOp{
				LiquidityPoolId: xdr.PoolId{2},
				Amount:          100,
				MinAmountA:      1,
				MinAmountB:      1,
			},
		}},
	} {
		_, include, err := filter.FilterTransaction(context.Background(),
			successfulTxWithMetaV3(issuer, []xdr.Operation{op}, poolChanges))
		tt.NoError(err, name)
		tt.True(include, name)

		_, include, err = filter.FilterTransaction(context.Background(),
			successfulTxWithMetaV3(issuer, []xdr.Operation{op}, unrelatedPoolChanges))
		tt.NoError(err, name)
		tt.False(include, name)
	}
}

func holderTrustLineEntry(asset xdr.Asset, balance xdr.Int64) xdr.LedgerEntry {
	return xdr.LedgerEntry{
		Data: xdr.LedgerEntryData{
			Type: xdr.LedgerEntryTypeTrustline,
			TrustLine: &xdr.TrustLineEntry{
				AccountId: xdr.MustAddress(testHolder),
				Asset:     asset.ToTrustLineAsset(),
				Balance:   balance,
				Limit:     1000,
			},
		},
	}
}

func TestAssetFilterMatchesTrustlineBalanceChange(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	holder := testHolder
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	other := xdr.MustNewCreditAsset("EURC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	_, include, err := filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(holder, nil,
			stateAndUpdated(holderTrustLineEntry(whitelisted, 100), holderTrustLineEntry(whitelisted, 200))))
	tt.NoError(err)
	tt.True(include)

	_, include, err = filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(holder, nil,
			stateAndUpdated(holderTrustLineEntry(other, 100), holderTrustLineEntry(other, 200))))
	tt.NoError(err)
	tt.False(include)
}

func TestAssetFilterMatchesFailedTransactionOnOperationBody(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	tx := successfulTxWithMetaV3(issuer, []xdr.Operation{{Body: xdr.OperationBody{
		Type: xdr.OperationTypePayment,
		PaymentOp: &xdr.PaymentOp{
			Destination: xdr.MustMuxedAddress(issuer),
			Asset:       whitelisted,
			Amount:      100,
		},
	}}}, nil)
	tx.Result.Result.Result.Code = xdr.TransactionResultCodeTxFailed
	tx.UnsafeMeta.V3.Operations = nil

	_, include, err := filter.FilterTransaction(context.Background(), tx)
	tt.NoError(err)
	tt.True(include)
}

func TestAssetFilterMatchesSACContractBalanceChange(t *testing.T) {
	tt := assert.New(t)
	whitelisted := xdr.MustNewCreditAsset("USDC", testIssuer)
	other := xdr.MustNewCreditAsset("EURC", testIssuer)
	filter := newTestAssetFilter(t, whitelisted)

	whitelistedContractID, err := whitelisted.ContractID(testNetworkPassphrase)
	tt.NoError(err)
	otherContractID, err := other.ContractID(testNetworkPassphrase)
	tt.NoError(err)
	holder := [32]byte{7}

	balanceCreated := func(assetContractID [32]byte) xdr.LedgerEntryChanges {
		entry := xdr.LedgerEntry{Data: sac.BalanceToContractData(assetContractID, holder, 100)}
		return xdr.LedgerEntryChanges{
			{Type: xdr.LedgerEntryChangeTypeLedgerEntryCreated, Created: &entry},
		}
	}

	_, include, err := filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(testIssuer, nil, balanceCreated(whitelistedContractID)))
	tt.NoError(err)
	tt.True(include)

	_, include, err = filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(testIssuer, nil, balanceCreated(otherContractID)))
	tt.NoError(err)
	tt.False(include)
}

func TestAssetFilterMatchesSACContractDeployment(t *testing.T) {
	tt := assert.New(t)
	whitelisted := xdr.MustNewCreditAsset("USDC", testIssuer)
	filter := newTestAssetFilter(t, whitelisted)

	metadataCreated := func(code string) xdr.LedgerEntryChanges {
		asset := xdr.MustNewCreditAsset(code, testIssuer)
		contractID, err := asset.ContractID(testNetworkPassphrase)
		require.NoError(t, err)
		data, err := sac.AssetToContractData(false, code, testIssuer, contractID)
		require.NoError(t, err)
		entry := xdr.LedgerEntry{Data: data}
		return xdr.LedgerEntryChanges{
			{Type: xdr.LedgerEntryChangeTypeLedgerEntryCreated, Created: &entry},
		}
	}

	_, include, err := filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(testIssuer, nil, metadataCreated("USDC")))
	tt.NoError(err)
	tt.True(include)

	_, include, err = filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(testIssuer, nil, metadataCreated("EURC")))
	tt.NoError(err)
	tt.False(include)
}
