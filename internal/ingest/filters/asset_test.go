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
	whitelisted := xdr.MustNewCreditAsset("USDC", testIssuer)
	other1 := xdr.MustNewCreditAsset("EURC", testIssuer)
	other2 := xdr.MustNewCreditAsset("GBPC", testIssuer)
	filter := newTestAssetFilter(t, whitelisted)

	pathPayments := func(path []xdr.Asset) map[string]xdr.Operation {
		return map[string]xdr.Operation{
			"strict send": {Body: xdr.OperationBody{
				Type: xdr.OperationTypePathPaymentStrictSend,
				PathPaymentStrictSendOp: &xdr.PathPaymentStrictSendOp{
					SendAsset:   other1,
					SendAmount:  100,
					Destination: xdr.MustMuxedAddress(testHolder),
					DestAsset:   other2,
					DestMin:     1,
					Path:        path,
				},
			}},
			"strict receive": {Body: xdr.OperationBody{
				Type: xdr.OperationTypePathPaymentStrictReceive,
				PathPaymentStrictReceiveOp: &xdr.PathPaymentStrictReceiveOp{
					SendAsset:   other1,
					SendMax:     100,
					Destination: xdr.MustMuxedAddress(testHolder),
					DestAsset:   other2,
					DestAmount:  1,
					Path:        path,
				},
			}},
		}
	}

	for name, op := range pathPayments([]xdr.Asset{whitelisted}) {
		isEnabled, include, err := filter.FilterTransaction(context.Background(),
			successfulTxWithMetaV3(testIssuer, []xdr.Operation{op}, nil))
		tt.NoError(err, name)
		tt.True(isEnabled, name)
		tt.True(include, name)
	}
	for name, op := range pathPayments([]xdr.Asset{xdr.MustNewNativeAsset()}) {
		_, include, err := filter.FilterTransaction(context.Background(),
			successfulTxWithMetaV3(testIssuer, []xdr.Operation{op}, nil))
		tt.NoError(err, name)
		tt.False(include, name)
	}
}

func newTestAssetFilter(t testing.TB, whitelist ...xdr.Asset) AssetFilter {
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

func created(entry xdr.LedgerEntry) xdr.LedgerEntryChanges {
	return xdr.LedgerEntryChanges{
		{Type: xdr.LedgerEntryChangeTypeLedgerEntryCreated, Created: &entry},
	}
}

func TestAssetFilterMatchesSetTrustLineFlags(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	setFlags := func(asset xdr.Asset) ingest.LedgerTransaction {
		return successfulTxWithMetaV3(issuer, []xdr.Operation{{Body: xdr.OperationBody{
			Type: xdr.OperationTypeSetTrustLineFlags,
			SetTrustLineFlagsOp: &xdr.SetTrustLineFlagsOp{
				Trustor:  xdr.MustAddress(issuer),
				Asset:    asset,
				SetFlags: xdr.Uint32(xdr.TrustLineFlagsAuthorizedFlag),
			},
		}}}, nil)
	}

	isEnabled, include, err := filter.FilterTransaction(context.Background(), setFlags(whitelisted))
	tt.NoError(err)
	tt.True(isEnabled)
	tt.True(include)

	_, include, err = filter.FilterTransaction(context.Background(), setFlags(xdr.MustNewCreditAsset("EURC", issuer)))
	tt.NoError(err)
	tt.False(include)
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

	feeBump := wrapInFeeBump(successfulTxWithMetaV3(issuer, []xdr.Operation{allowTrust(nil)}, nil))
	feeBump.Envelope.FeeBump.Tx.FeeSource = xdr.MustMuxedAddress(other)
	_, include, err := filter.FilterTransaction(context.Background(), feeBump)
	tt.NoError(err)
	tt.True(include, "fee-bump: the issuer is the inner transaction source, not the fee source")

	feeSourceIsIssuer := wrapInFeeBump(successfulTxWithMetaV3(other, []xdr.Operation{allowTrust(nil)}, nil))
	feeSourceIsIssuer.Envelope.FeeBump.Tx.FeeSource = issuerMuxed
	_, include, err = filter.FilterTransaction(context.Background(), feeSourceIsIssuer)
	tt.NoError(err)
	tt.False(include, "fee-bump: the fee source is not the issuer of an AllowTrust")

	notIssuer := successfulTxWithMetaV3(other, []xdr.Operation{allowTrust(nil)}, nil)
	_, include, err = filter.FilterTransaction(context.Background(), notIssuer)
	tt.NoError(err)
	tt.False(include)
}

func TestAssetFilterMatchesRevokeSponsorshipOfTrustline(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	other := testHolder
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	revoke := func(asset xdr.Asset) ingest.LedgerTransaction {
		key := xdr.LedgerKey{
			Type: xdr.LedgerEntryTypeTrustline,
			TrustLine: &xdr.LedgerKeyTrustLine{
				AccountId: xdr.MustAddress(other),
				Asset:     asset.ToTrustLineAsset(),
			},
		}
		return successfulTxWithMetaV3(issuer, []xdr.Operation{{Body: xdr.OperationBody{
			Type: xdr.OperationTypeRevokeSponsorship,
			RevokeSponsorshipOp: &xdr.RevokeSponsorshipOp{
				Type:      xdr.RevokeSponsorshipTypeRevokeSponsorshipLedgerEntry,
				LedgerKey: &key,
			},
		}}}, nil)
	}

	isEnabled, include, err := filter.FilterTransaction(context.Background(), revoke(whitelisted))
	tt.NoError(err)
	tt.True(isEnabled)
	tt.True(include)

	_, include, err = filter.FilterTransaction(context.Background(), revoke(xdr.MustNewCreditAsset("EURC", issuer)))
	tt.NoError(err)
	tt.False(include)
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

func TestAssetFilterMatchesClaimableBalanceClaimAndClawback(t *testing.T) {
	tt := assert.New(t)
	issuer := testIssuer
	whitelisted := xdr.MustNewCreditAsset("USDC", issuer)
	other := xdr.MustNewCreditAsset("EURC", issuer)
	filter := newTestAssetFilter(t, whitelisted)

	balanceID := xdr.ClaimableBalanceId{
		Type: xdr.ClaimableBalanceIdTypeClaimableBalanceIdTypeV0,
		V0:   &xdr.Hash{1},
	}
	for name, op := range map[string]xdr.Operation{
		"claim": {Body: xdr.OperationBody{
			Type:                    xdr.OperationTypeClaimClaimableBalance,
			ClaimClaimableBalanceOp: &xdr.ClaimClaimableBalanceOp{BalanceId: balanceID},
		}},
		"clawback": {Body: xdr.OperationBody{
			Type:                       xdr.OperationTypeClawbackClaimableBalance,
			ClawbackClaimableBalanceOp: &xdr.ClawbackClaimableBalanceOp{BalanceId: balanceID},
		}},
	} {
		_, include, err := filter.FilterTransaction(context.Background(),
			successfulTxWithMetaV3(issuer, []xdr.Operation{op}, stateAndRemoved(claimableBalanceEntry(whitelisted))))
		tt.NoError(err, name)
		tt.True(include, name)

		_, include, err = filter.FilterTransaction(context.Background(),
			successfulTxWithMetaV3(issuer, []xdr.Operation{op}, stateAndRemoved(claimableBalanceEntry(other))))
		tt.NoError(err, name)
		tt.False(include, name)
	}
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

func sacContractID(t testing.TB, asset xdr.Asset) xdr.ContractId {
	id, err := asset.ContractID(testNetworkPassphrase)
	require.NoError(t, err)
	return xdr.ContractId(id)
}

func sacBalanceEntry(contractID xdr.ContractId) xdr.LedgerEntry {
	holder := [32]byte{7}
	return xdr.LedgerEntry{Data: sac.BalanceToContractData(contractID, holder, 100)}
}

func TestAssetFilterMatchesSACContractBalanceChange(t *testing.T) {
	tt := assert.New(t)
	native := xdr.MustNewNativeAsset()
	usdc := xdr.MustNewCreditAsset("USDC", testIssuer)
	eurc := xdr.MustNewCreditAsset("EURC", testIssuer)

	for _, tc := range []struct {
		name      string
		whitelist xdr.Asset
		balanceOf xdr.Asset
		include   bool
	}{
		{"usdc balance, usdc whitelisted", usdc, usdc, true},
		{"eurc balance, usdc whitelisted", usdc, eurc, false},
		{"native balance, native whitelisted", native, native, true},
		{"usdc balance, native whitelisted", native, usdc, false},
	} {
		filter := newTestAssetFilter(t, tc.whitelist)
		_, include, err := filter.FilterTransaction(context.Background(),
			successfulTxWithMetaV3(testIssuer, nil, created(sacBalanceEntry(sacContractID(t, tc.balanceOf)))))
		tt.NoError(err, tc.name)
		tt.Equal(tc.include, include, tc.name)
	}
}

func invokeContract(contractID xdr.ContractId) xdr.Operation {
	return xdr.Operation{Body: xdr.OperationBody{
		Type: xdr.OperationTypeInvokeHostFunction,
		InvokeHostFunctionOp: &xdr.InvokeHostFunctionOp{
			HostFunction: xdr.HostFunction{
				Type: xdr.HostFunctionTypeHostFunctionTypeInvokeContract,
				InvokeContract: &xdr.InvokeContractArgs{
					ContractAddress: xdr.ScAddress{
						Type:       xdr.ScAddressTypeScAddressTypeContract,
						ContractId: &contractID,
					},
					FunctionName: "transfer",
				},
			},
		},
	}}
}

func accountEntry(address string, balance xdr.Int64) xdr.LedgerEntry {
	return xdr.LedgerEntry{Data: xdr.LedgerEntryData{
		Type: xdr.LedgerEntryTypeAccount,
		Account: &xdr.AccountEntry{
			AccountId: xdr.MustAddress(address),
			Balance:   balance,
		},
	}}
}

func TestAssetFilterMatchesSACInvocationOnOperationBody(t *testing.T) {
	tt := assert.New(t)
	native := xdr.MustNewNativeAsset()
	usdc := xdr.MustNewCreditAsset("USDC", testIssuer)
	eurc := xdr.MustNewCreditAsset("EURC", testIssuer)

	// A failed call writes no operation meta. Only the operation body can match.
	failedInvoke := func(contractID xdr.ContractId) ingest.LedgerTransaction {
		tx := successfulTxWithMetaV3(testHolder, []xdr.Operation{invokeContract(contractID)}, nil)
		tx.Result.Result.Result.Code = xdr.TransactionResultCodeTxFailed
		tx.UnsafeMeta.V3.Operations = nil
		return tx
	}
	filter := newTestAssetFilter(t, usdc)
	_, include, err := filter.FilterTransaction(context.Background(), failedInvoke(sacContractID(t, usdc)))
	tt.NoError(err)
	tt.True(include, "failed call to the whitelisted SAC")
	_, include, err = filter.FilterTransaction(context.Background(), failedInvoke(sacContractID(t, eurc)))
	tt.NoError(err)
	tt.False(include, "failed call to another SAC")

	// A native transfer between two account addresses changes only account
	// entries, which the filter does not read. The operation body matches.
	nativeTransfer := successfulTxWithMetaV3(testHolder, []xdr.Operation{invokeContract(sacContractID(t, native))},
		append(stateAndUpdated(accountEntry(testHolder, 200), accountEntry(testHolder, 100)),
			stateAndUpdated(accountEntry(testIssuer, 100), accountEntry(testIssuer, 200))...))
	_, include, err = newTestAssetFilter(t, native).FilterTransaction(context.Background(), nativeTransfer)
	tt.NoError(err)
	tt.True(include, "native SAC transfer between two G accounts")
	_, include, err = filter.FilterTransaction(context.Background(), nativeTransfer)
	tt.NoError(err)
	tt.False(include, "native SAC transfer when only USDC is whitelisted")
}

func TestAssetFilterMatchesSACDeploymentOnOperationBody(t *testing.T) {
	tt := assert.New(t)
	usdc := xdr.MustNewCreditAsset("USDC", testIssuer)
	eurc := xdr.MustNewCreditAsset("EURC", testIssuer)
	filter := newTestAssetFilter(t, usdc)

	deploy := func(asset xdr.Asset) ingest.LedgerTransaction {
		fn := xdr.HostFunction{
			Type: xdr.HostFunctionTypeHostFunctionTypeCreateContract,
			CreateContract: &xdr.CreateContractArgs{
				ContractIdPreimage: xdr.ContractIdPreimage{
					Type:      xdr.ContractIdPreimageTypeContractIdPreimageFromAsset,
					FromAsset: &asset,
				},
				Executable: xdr.ContractExecutable{Type: xdr.ContractExecutableTypeContractExecutableStellarAsset},
			},
		}
		tx := successfulTxWithMetaV3(testHolder, []xdr.Operation{{Body: xdr.OperationBody{
			Type:                 xdr.OperationTypeInvokeHostFunction,
			InvokeHostFunctionOp: &xdr.InvokeHostFunctionOp{HostFunction: fn},
		}}}, nil)
		tx.Result.Result.Result.Code = xdr.TransactionResultCodeTxFailed
		tx.UnsafeMeta.V3.Operations = nil
		return tx
	}

	_, include, err := filter.FilterTransaction(context.Background(), deploy(usdc))
	tt.NoError(err)
	tt.True(include)
	_, include, err = filter.FilterTransaction(context.Background(), deploy(eurc))
	tt.NoError(err)
	tt.False(include)
}

func TestAssetFilterMatchesSorobanFootprint(t *testing.T) {
	tt := assert.New(t)
	usdc := xdr.MustNewCreditAsset("USDC", testIssuer)
	eurc := xdr.MustNewCreditAsset("EURC", testIssuer)
	filter := newTestAssetFilter(t, usdc)

	// ExtendFootprintTTL names the entries only in the footprint. The meta
	// holds TTL entries, which carry a key hash and no contract id.
	extendTTL := func(contractID xdr.ContractId) ingest.LedgerTransaction {
		entry := sacBalanceEntry(contractID)
		key, err := entry.LedgerKey()
		require.NoError(t, err)
		tx := successfulTxWithMetaV3(testHolder, []xdr.Operation{{Body: xdr.OperationBody{
			Type:                 xdr.OperationTypeExtendFootprintTtl,
			ExtendFootprintTtlOp: &xdr.ExtendFootprintTtlOp{ExtendTo: 1000},
		}}}, nil)
		tx.Envelope.V1.Tx.Ext = xdr.TransactionExt{
			V: 1,
			SorobanData: &xdr.SorobanTransactionData{
				Resources: xdr.SorobanResources{
					Footprint: xdr.LedgerFootprint{ReadOnly: []xdr.LedgerKey{key}},
				},
			},
		}
		return tx
	}

	_, include, err := filter.FilterTransaction(context.Background(), extendTTL(sacContractID(t, usdc)))
	tt.NoError(err)
	tt.True(include)
	_, include, err = filter.FilterTransaction(context.Background(), wrapInFeeBump(extendTTL(sacContractID(t, usdc))))
	tt.NoError(err)
	tt.True(include, "fee-bump")
	_, include, err = filter.FilterTransaction(context.Background(), extendTTL(sacContractID(t, eurc)))
	tt.NoError(err)
	tt.False(include)
}

func TestAssetFilterRejectsUnknownMetaVersion(t *testing.T) {
	tt := assert.New(t)
	filter := newTestAssetFilter(t, xdr.MustNewCreditAsset("USDC", testIssuer))

	tx := successfulTxWithMetaV3(testHolder, nil, nil)
	tx.UnsafeMeta = xdr.TransactionMeta{V: 5}
	_, include, err := filter.FilterTransaction(context.Background(), tx)
	tt.EqualError(err, "unsupported transaction meta version 5")
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
		return created(xdr.LedgerEntry{Data: data})
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

func TestAssetFilterMatchesWhitelistEntryStoredInNonCanonicalForm(t *testing.T) {
	tt := assert.New(t)
	filter := NewAssetFilter(testNetworkPassphrase)
	tt.NoError(filter.RefreshAssetFilter(&history.AssetFilterConfig{
		Whitelist:    []string{"NATIVE"},
		Enabled:      true,
		LastModified: 1,
	}))

	payment := func(asset xdr.Asset) ingest.LedgerTransaction {
		return successfulTxWithMetaV3(testIssuer, []xdr.Operation{{Body: xdr.OperationBody{
			Type: xdr.OperationTypePayment,
			PaymentOp: &xdr.PaymentOp{
				Destination: xdr.MustMuxedAddress(testHolder),
				Asset:       asset,
				Amount:      100,
			},
		}}}, nil)
	}

	_, include, err := filter.FilterTransaction(context.Background(), payment(xdr.MustNewNativeAsset()))
	tt.NoError(err)
	tt.True(include)

	_, include, err = filter.FilterTransaction(context.Background(), payment(xdr.MustNewCreditAsset("USDC", testIssuer)))
	tt.NoError(err)
	tt.False(include)
}

func TestAssetFilterKeepsMalformedWhitelistEntryEnabled(t *testing.T) {
	tt := assert.New(t)
	filter := NewAssetFilter(testNetworkPassphrase)
	tt.NoError(filter.RefreshAssetFilter(&history.AssetFilterConfig{
		Whitelist:    []string{"not-an-asset"},
		Enabled:      true,
		LastModified: 1,
	}))

	isEnabled, include, err := filter.FilterTransaction(context.Background(),
		successfulTxWithMetaV3(testIssuer, nil, nil))
	tt.NoError(err)
	tt.True(isEnabled)
	tt.False(include)
}

func TestAssetFilterMatchesChangesInEveryMetaVersion(t *testing.T) {
	tt := assert.New(t)
	whitelisted := xdr.MustNewCreditAsset("USDC", testIssuer)
	other := xdr.MustNewCreditAsset("EURC", testIssuer)
	filter := newTestAssetFilter(t, whitelisted)

	metas := func(asset xdr.Asset) map[string]xdr.TransactionMeta {
		changes := stateAndUpdated(holderTrustLineEntry(asset, 100), holderTrustLineEntry(asset, 200))
		v0Operations := []xdr.OperationMeta{{Changes: changes}}
		return map[string]xdr.TransactionMeta{
			"v0":        {V: 0, Operations: &v0Operations},
			"v1":        {V: 1, V1: &xdr.TransactionMetaV1{Operations: []xdr.OperationMeta{{Changes: changes}}}},
			"v2 before": {V: 2, V2: &xdr.TransactionMetaV2{TxChangesBefore: changes}},
			"v3 after":  {V: 3, V3: &xdr.TransactionMetaV3{TxChangesAfter: changes}},
			"v4":        {V: 4, V4: &xdr.TransactionMetaV4{Operations: []xdr.OperationMetaV2{{Changes: changes}}}},
		}
	}

	for name, meta := range metas(whitelisted) {
		tx := successfulTxWithMetaV3(testHolder, nil, nil)
		tx.UnsafeMeta = meta
		_, include, err := filter.FilterTransaction(context.Background(), tx)
		tt.NoError(err, name)
		tt.True(include, name)
	}
	for name, meta := range metas(other) {
		tx := successfulTxWithMetaV3(testHolder, nil, nil)
		tx.UnsafeMeta = meta
		_, include, err := filter.FilterTransaction(context.Background(), tx)
		tt.NoError(err, name)
		tt.False(include, name)
	}
}

func TestAssetKeyAgreesWithCanonicalString(t *testing.T) {
	tt := assert.New(t)
	var usdc12Code [12]byte
	copy(usdc12Code[:], "USDC")
	issuer := xdr.MustAddress(testIssuer)
	assets := []xdr.Asset{
		xdr.MustNewNativeAsset(),
		xdr.MustNewCreditAsset("USDC", testIssuer),
		xdr.MustNewCreditAsset("USDC", testHolder),
		xdr.MustNewCreditAsset("USD", testIssuer),
		xdr.MustNewCreditAsset("USDCOIN", testIssuer),
		{
			Type:       xdr.AssetTypeAssetTypeCreditAlphanum12,
			AlphaNum12: &xdr.AlphaNum12{AssetCode: usdc12Code, Issuer: issuer},
		},
	}
	for _, a := range assets {
		for _, b := range assets {
			tt.Equal(a.StringCanonical() == b.StringCanonical(), newAssetKey(a) == newAssetKey(b),
				"%s vs %s", a.StringCanonical(), b.StringCanonical())
		}
	}
}
