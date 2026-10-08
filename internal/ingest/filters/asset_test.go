package filters

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go-stellar-sdk/ingest"
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
	filter := NewAssetFilter()
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
	filter := NewAssetFilter()
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
	filter := NewAssetFilter()
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

	filter := NewAssetFilter()
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

	filter := NewAssetFilter()
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

	filter := NewAssetFilter()
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
