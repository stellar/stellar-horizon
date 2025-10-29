package history

import (
	"fmt"
	"math"
	"testing"

	"github.com/guregu/null"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"

	"github.com/stellar/go/ingest"
	"github.com/stellar/go/xdr"
)

func TestTransactionToMap_muxed(t *testing.T) {
	innerSource := xdr.MuxedAccount{
		Type: xdr.CryptoKeyTypeKeyTypeMuxedEd25519,
		Med25519: &xdr.MuxedAccountMed25519{
			Id:      1,
			Ed25519: xdr.Uint256{3, 2, 1},
		},
	}
	innerAccountID := innerSource.ToAccountId()
	feeSource := xdr.MuxedAccount{
		Type: xdr.CryptoKeyTypeKeyTypeMuxedEd25519,
		Med25519: &xdr.MuxedAccountMed25519{
			Id:      1,
			Ed25519: xdr.Uint256{0, 1, 2},
		},
	}
	feeSourceAccountID := feeSource.ToAccountId()
	tx := ingest.LedgerTransaction{
		Index: 1,
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTxFeeBump,
			FeeBump: &xdr.FeeBumpTransactionEnvelope{
				Tx: xdr.FeeBumpTransaction{
					FeeSource: feeSource,
					Fee:       200,
					InnerTx: xdr.FeeBumpTransactionInnerTx{
						Type: xdr.EnvelopeTypeEnvelopeTypeTx,
						V1: &xdr.TransactionV1Envelope{
							Tx: xdr.Transaction{
								SourceAccount: innerSource,
								Operations: []xdr.Operation{
									{
										SourceAccount: &innerSource,
										Body: xdr.OperationBody{
											Type: xdr.OperationTypePayment,
											PaymentOp: &xdr.PaymentOp{
												Destination: innerSource,
												Asset:       xdr.Asset{Type: xdr.AssetTypeAssetTypeNative},
												Amount:      100,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Result: xdr.TransactionResultPair{
			TransactionHash: xdr.Hash{1, 2, 3},
			Result: xdr.TransactionResult{
				Result: xdr.TransactionResultResult{
					Code: xdr.TransactionResultCodeTxFeeBumpInnerSuccess,
					InnerResultPair: &xdr.InnerTransactionResultPair{
						TransactionHash: xdr.Hash{3, 2, 1},
						Result: xdr.InnerTransactionResult{
							Result: xdr.InnerTransactionResultResult{
								Results: &[]xdr.OperationResult{},
							},
						},
					},
					Results: &[]xdr.OperationResult{},
				},
			},
		},
		UnsafeMeta: xdr.TransactionMeta{
			V:          1,
			Operations: &[]xdr.OperationMeta{},
			V1: &xdr.TransactionMetaV1{
				TxChanges:  []xdr.LedgerEntryChange{},
				Operations: []xdr.OperationMeta{},
			},
		},
	}
	b := &transactionBatchInsertBuilder{
		encodingBuffer: xdr.NewEncodingBuffer(),
	}
	row, err := transactionToRow(tx, 20, b.encodingBuffer)
	assert.NoError(t, err)

	assert.Equal(t, innerAccountID.Address(), row.Account)

	assert.Equal(t, feeSourceAccountID.Address(), row.FeeAccount.String)

	assert.Equal(t, feeSource.Address(), row.FeeAccountMuxed.String)
}

func TestTransactionToMap_SourceMuxedAndFeeSourceUnmuxed(t *testing.T) {
	innerSource := xdr.MuxedAccount{
		Type: xdr.CryptoKeyTypeKeyTypeMuxedEd25519,
		Med25519: &xdr.MuxedAccountMed25519{
			Id:      1,
			Ed25519: xdr.Uint256{3, 2, 1},
		},
	}
	innerAccountID := innerSource.ToAccountId()
	feeSource := xdr.MuxedAccount{
		Type:    xdr.CryptoKeyTypeKeyTypeEd25519,
		Ed25519: &xdr.Uint256{0, 1, 2},
	}
	tx := ingest.LedgerTransaction{
		Index: 1,
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTxFeeBump,
			FeeBump: &xdr.FeeBumpTransactionEnvelope{
				Tx: xdr.FeeBumpTransaction{
					FeeSource: feeSource,
					Fee:       200,
					InnerTx: xdr.FeeBumpTransactionInnerTx{
						Type: xdr.EnvelopeTypeEnvelopeTypeTx,
						V1: &xdr.TransactionV1Envelope{
							Tx: xdr.Transaction{
								SourceAccount: innerSource,
								Operations: []xdr.Operation{
									{
										SourceAccount: &innerSource,
										Body: xdr.OperationBody{
											Type: xdr.OperationTypePayment,
											PaymentOp: &xdr.PaymentOp{
												Destination: innerSource,
												Asset:       xdr.Asset{Type: xdr.AssetTypeAssetTypeNative},
												Amount:      100,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Result: xdr.TransactionResultPair{
			TransactionHash: xdr.Hash{1, 2, 3},
			Result: xdr.TransactionResult{
				Result: xdr.TransactionResultResult{
					Code: xdr.TransactionResultCodeTxFeeBumpInnerSuccess,
					InnerResultPair: &xdr.InnerTransactionResultPair{
						TransactionHash: xdr.Hash{3, 2, 1},
						Result: xdr.InnerTransactionResult{
							Result: xdr.InnerTransactionResultResult{
								Results: &[]xdr.OperationResult{},
							},
						},
					},
					Results: &[]xdr.OperationResult{},
				},
			},
		},
		UnsafeMeta: xdr.TransactionMeta{
			V:          1,
			Operations: &[]xdr.OperationMeta{},
			V1: &xdr.TransactionMetaV1{
				TxChanges:  []xdr.LedgerEntryChange{},
				Operations: []xdr.OperationMeta{},
			},
		},
	}
	b := &transactionBatchInsertBuilder{
		encodingBuffer: xdr.NewEncodingBuffer(),
	}
	row, err := transactionToRow(tx, 20, b.encodingBuffer)
	assert.NoError(t, err)

	assert.Equal(t, innerAccountID.Address(), row.Account)

	assert.Equal(t, feeSource.Address(), row.FeeAccount.String)

	assert.False(t, row.FeeAccountMuxed.Valid)
}

func TestTransactionToMap_Preconditions(t *testing.T) {
	source := xdr.MuxedAccount{
		Type:    xdr.CryptoKeyTypeKeyTypeEd25519,
		Ed25519: &xdr.Uint256{3, 2, 1},
	}
	minSeqNum := xdr.SequenceNumber(math.MaxInt64)
	signerKey := xdr.SignerKey{
		Type:    xdr.SignerKeyTypeSignerKeyTypeEd25519,
		Ed25519: source.Ed25519,
	}
	tx := ingest.LedgerTransaction{
		Index: 1,
		Envelope: xdr.TransactionEnvelope{
			Type: xdr.EnvelopeTypeEnvelopeTypeTx,
			V1: &xdr.TransactionV1Envelope{
				Tx: xdr.Transaction{
					SourceAccount: source,
					Operations: []xdr.Operation{
						{
							SourceAccount: &source,
							Body: xdr.OperationBody{
								Type: xdr.OperationTypePayment,
								PaymentOp: &xdr.PaymentOp{
									Destination: source,
									Asset:       xdr.Asset{Type: xdr.AssetTypeAssetTypeNative},
									Amount:      100,
								},
							},
						},
					},
					Cond: xdr.Preconditions{
						Type: xdr.PreconditionTypePrecondV2,
						V2: &xdr.PreconditionsV2{
							// The important bit.
							TimeBounds: &xdr.TimeBounds{
								MinTime: 1000,
								MaxTime: 2000,
							},
							LedgerBounds: &xdr.LedgerBounds{
								MinLedger: 5,
								MaxLedger: 10,
							},
							MinSeqNum:       &minSeqNum,
							MinSeqAge:       xdr.Duration(math.MaxUint64),
							MinSeqLedgerGap: xdr.Uint32(3),
							ExtraSigners:    []xdr.SignerKey{signerKey},
						},
					},
				},
			},
		},
		Result: xdr.TransactionResultPair{
			TransactionHash: xdr.Hash{1, 2, 3},
			Result: xdr.TransactionResult{
				Result: xdr.TransactionResultResult{
					Code: xdr.TransactionResultCodeTxFeeBumpInnerSuccess,
					InnerResultPair: &xdr.InnerTransactionResultPair{
						TransactionHash: xdr.Hash{3, 2, 1},
						Result: xdr.InnerTransactionResult{
							Result: xdr.InnerTransactionResultResult{
								Results: &[]xdr.OperationResult{},
							},
						},
					},
					Results: &[]xdr.OperationResult{},
				},
			},
		},
		UnsafeMeta: xdr.TransactionMeta{
			V:          1,
			Operations: &[]xdr.OperationMeta{},
			V1: &xdr.TransactionMetaV1{
				TxChanges:  []xdr.LedgerEntryChange{},
				Operations: []xdr.OperationMeta{},
			},
		},
	}
	row, err := transactionToRow(tx, 20, xdr.NewEncodingBuffer())
	assert.NoError(t, err)

	assert.Equal(t, null.IntFrom(1000), row.TimeBounds.Lower)
	assert.Equal(t, null.IntFrom(2000), row.TimeBounds.Upper)

	assert.Equal(t, null.IntFrom(5), row.LedgerBounds.MinLedger)
	assert.Equal(t, null.IntFrom(10), row.LedgerBounds.MaxLedger)

	assert.Equal(t, null.IntFrom(int64(minSeqNum)), row.MinAccountSequence)
	assert.Equal(t, null.StringFrom(fmt.Sprint(uint64(math.MaxUint64))), row.MinAccountSequenceAge)
	assert.Equal(t, null.IntFrom(3), row.MinAccountSequenceLedgerGap)
	assert.Equal(t, pq.StringArray{signerKey.Address()}, row.ExtraSigners)
}
