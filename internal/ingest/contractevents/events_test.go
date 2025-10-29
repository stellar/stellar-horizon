package contractevents

import (
	"github.com/stellar/go/ingest"
	"math/big"
	"testing"

	"github.com/stellar/go/keypair"
	"github.com/stellar/go/strkey"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const passphrase = "passphrase"

var (
	randomIssuer     = keypair.MustRandom()
	randomAsset      = xdr.MustNewCreditAsset("TESTING", randomIssuer.Address())
	randomAccount    = keypair.MustRandom().Address()
	zeroContractHash = xdr.Hash([32]byte{})
	zeroContract     = strkey.MustEncode(strkey.VersionByteContract, zeroContractHash[:])
)

// Test fixture structure
type testcase struct {
	name           string
	txMetaVersion  int32
	eventType      EventType
	topics         []xdr.ScVal
	data           xdr.ScVal
	asset          xdr.Asset
	contractID     *xdr.ContractId
	expectedResult *StellarAssetContractEvent
	expectedError  string
}

func TestStellarAssetContractEventParsing(t *testing.T) {
	testCases := []testcase{
		// ===== VALID V3 EVENTS =====
		{
			name:          "Valid V3 transfer event",
			txMetaVersion: 3,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:       makeBigAmount(big.NewInt(1000)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeTransfer,
				Asset:  randomAsset,
				From:   randomAccount,
				To:     zeroContract,
				Amount: xdr.Int128Parts{Lo: 1000, Hi: 0},
			},
		},
		{
			name:          "Valid V3 mint event with admin address",
			txMetaVersion: 3,
			eventType:     EventTypeMint,
			topics: []xdr.ScVal{
				makeSymbol("mint"),
				makeAddress(randomAccount), // admin (ignored)
				makeAddress(zeroContract),  // to
				makeAsset(randomAsset),
			},
			data:       makeBigAmount(big.NewInt(2000)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeMint,
				Asset:  randomAsset,
				To:     zeroContract,
				Amount: xdr.Int128Parts{Lo: 2000, Hi: 0},
			},
		},
		{
			name:          "Valid V3 clawback event with admin address",
			txMetaVersion: 3,
			eventType:     EventTypeClawback,
			topics: []xdr.ScVal{
				makeSymbol("clawback"),
				makeAddress(randomAccount), // admin (ignored)
				makeAddress(zeroContract),  // from
				makeAsset(randomAsset),
			},
			data:       makeBigAmount(big.NewInt(3000)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeClawback,
				Asset:  randomAsset,
				From:   zeroContract,
				Amount: xdr.Int128Parts{Lo: 3000, Hi: 0},
			},
		},
		{
			name:          "Valid V3 burn event",
			txMetaVersion: 3,
			eventType:     EventTypeBurn,
			topics: []xdr.ScVal{
				makeSymbol("burn"),
				makeAddress(randomAccount), // from
				makeAsset(randomAsset),
			},
			data:       makeBigAmount(big.NewInt(4000)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeBurn,
				Asset:  randomAsset,
				From:   randomAccount,
				Amount: xdr.Int128Parts{Lo: 4000, Hi: 0},
			},
		},
		{
			name:          "Valid V3 transfer with native asset",
			txMetaVersion: 3,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(xdr.MustNewNativeAsset()),
			},
			data:       makeBigAmount(big.NewInt(5000)),
			asset:      xdr.MustNewNativeAsset(),
			contractID: mustGetContractID(xdr.MustNewNativeAsset()),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeTransfer,
				Asset:  xdr.MustNewNativeAsset(),
				From:   randomAccount,
				To:     zeroContract,
				Amount: xdr.Int128Parts{Lo: 5000, Hi: 0},
			},
		},

		// ===== VALID V4 EVENTS =====
		{
			name:          "Valid V4 transfer event (same as V3)",
			txMetaVersion: 4,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:       makeBigAmount(big.NewInt(1000)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeTransfer,
				Asset:  randomAsset,
				From:   randomAccount,
				To:     zeroContract,
				Amount: xdr.Int128Parts{Lo: 1000, Hi: 0},
			},
		},
		{
			name:          "Valid V4 mint event without admin address",
			txMetaVersion: 4,
			eventType:     EventTypeMint,
			topics: []xdr.ScVal{
				makeSymbol("mint"),
				makeAddress(zeroContract), // to (no admin in V4)
				makeAsset(randomAsset),
			},
			data:       makeBigAmount(big.NewInt(2000)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeMint,
				Asset:  randomAsset,
				To:     zeroContract,
				Amount: xdr.Int128Parts{Lo: 2000, Hi: 0},
			},
		},
		{
			name:          "Valid V4 clawback event without admin address",
			txMetaVersion: 4,
			eventType:     EventTypeClawback,
			topics: []xdr.ScVal{
				makeSymbol("clawback"),
				makeAddress(zeroContract), // from (no admin in V4)
				makeAsset(randomAsset),
			},
			data:       makeBigAmount(big.NewInt(3000)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeClawback,
				Asset:  randomAsset,
				From:   zeroContract,
				Amount: xdr.Int128Parts{Lo: 3000, Hi: 0},
			},
		},
		{
			name:          "Valid V4 burn event (same as V3)",
			txMetaVersion: 4,
			eventType:     EventTypeBurn,
			topics: []xdr.ScVal{
				makeSymbol("burn"),
				makeAddress(randomAccount),
				makeAsset(randomAsset),
			},
			data:       makeBigAmount(big.NewInt(4000)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:   EventTypeBurn,
				Asset:  randomAsset,
				From:   randomAccount,
				Amount: xdr.Int128Parts{Lo: 4000, Hi: 0},
			},
		},
		{
			name:          "Valid V4 transfer with uint64 memo",
			txMetaVersion: 4,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:       makeV4MapData(big.NewInt(1000), xdr.MemoID(12345)),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:            EventTypeTransfer,
				Asset:           randomAsset,
				From:            randomAccount,
				To:              zeroContract,
				Amount:          xdr.Int128Parts{Lo: 1000, Hi: 0},
				DestinationMemo: xdr.MemoID(12345),
			},
		},
		{
			name:          "Valid V4 transfer with text memo",
			txMetaVersion: 4,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:       makeV4MapData(big.NewInt(1000), xdr.MemoText("hello")),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:            EventTypeTransfer,
				Asset:           randomAsset,
				From:            randomAccount,
				To:              zeroContract,
				Amount:          xdr.Int128Parts{Lo: 1000, Hi: 0},
				DestinationMemo: xdr.MemoText("hello"),
			},
		},
		{
			name:          "Valid V4 transfer with hash memo",
			txMetaVersion: 4,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:       makeV4MapData(big.NewInt(1000), xdr.MemoHash([32]byte{1, 2, 3, 4})),
			asset:      randomAsset,
			contractID: mustGetContractID(randomAsset),
			expectedResult: &StellarAssetContractEvent{
				Type:            EventTypeTransfer,
				Asset:           randomAsset,
				From:            randomAccount,
				To:              zeroContract,
				Amount:          xdr.Int128Parts{Lo: 1000, Hi: 0},
				DestinationMemo: xdr.MemoHash([32]byte{1, 2, 3, 4}),
			},
		},

		// ===== INVALID EVENTS =====
		{
			name:          "Unsupported transaction meta version",
			txMetaVersion: 5,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:          makeBigAmount(big.NewInt(1000)),
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "tx meta version not supported",
		},
		{
			name:          "V3 event with insufficient topics",
			txMetaVersion: 3,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"), // Only 1 topic, need at least 3
			},
			data:          makeBigAmount(big.NewInt(1000)),
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "event was not from a Stellar asset Contract",
		},
		{
			name:          "V4 mint with V3 format (too many topics)",
			txMetaVersion: 4,
			eventType:     EventTypeMint,
			topics: []xdr.ScVal{
				makeSymbol("mint"),
				makeAddress(randomAccount), // admin (should not be in V4)
				makeAddress(zeroContract),  // to
				makeAsset(randomAsset),
			},
			data:          makeBigAmount(big.NewInt(2000)),
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "mint event requires 3 topics",
		},
		{
			name:          "Contract ID doesn't match asset",
			txMetaVersion: 3,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:          makeBigAmount(big.NewInt(1000)),
			asset:         randomAsset,
			contractID:    mustGetContractID(xdr.MustNewNativeAsset()), // Wrong contract ID
			expectedError: "contract ID doesn't match asset + passphrase",
		},
		{
			name:          "Unknown event type",
			txMetaVersion: 3,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("unknown_event"), // Unknown event type
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:          makeBigAmount(big.NewInt(1000)),
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "event was not from a Stellar asset Contract",
		},
		{
			name:          "Non-address topic where address expected",
			txMetaVersion: 3,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeSymbol("not_an_address"), // Should be address
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data:          makeBigAmount(big.NewInt(1000)),
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "invalid from address",
		},
		{
			name:          "V4 map data insufficient elements",
			txMetaVersion: 4,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data: func() xdr.ScVal {
				mapData := &xdr.ScMap{}
				return xdr.ScVal{
					Type: xdr.ScValTypeScvMap,
					Map:  &mapData,
				}
			}(),
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "failed to parse V4 map data: expected exactly 2 elements in map data",
		},
		{
			name:          "V4 map data - missing amount",
			txMetaVersion: 4,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data: func() xdr.ScVal {
				mapData := &xdr.ScMap{
					{
						Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &[]xdr.ScSymbol{"to_muxed_id"}[0]},
						Val: xdr.ScVal{Type: xdr.ScValTypeScvU64, U64: &[]xdr.Uint64{12345}[0]},
					},
					{
						Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &[]xdr.ScSymbol{"not_amount"}[0]},
						Val: xdr.ScVal{Type: xdr.ScValTypeScvU64, U64: &[]xdr.Uint64{12345}[0]},
					},
				}
				return xdr.ScVal{
					Type: xdr.ScValTypeScvMap,
					Map:  &mapData,
				}
			}(),
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "amount field not found in map",
		},
		{
			name:          "V4 map data - missing muxed id",
			txMetaVersion: 4,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data: func() xdr.ScVal {
				mapData := &xdr.ScMap{
					{
						Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &[]xdr.ScSymbol{"tooo_muxed_id"}[0]},
						Val: xdr.ScVal{Type: xdr.ScValTypeScvU64, U64: &[]xdr.Uint64{12345}[0]},
					},
					{
						Key: xdr.ScVal{Type: xdr.ScValTypeScvSymbol, Sym: &[]xdr.ScSymbol{"amount"}[0]},
						Val: makeBigAmount(big.NewInt(1000)),
					},
				}
				return xdr.ScVal{
					Type: xdr.ScValTypeScvMap,
					Map:  &mapData,
				}
			}(),
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "failed to parse V4 map data: to_muxed_id field not found in map",
		},
		{
			name:          "V3 Invalid amount data type",
			txMetaVersion: 3,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data: xdr.ScVal{
				Type: xdr.ScValTypeScvU64, // Should be i128
				U64:  &[]xdr.Uint64{1000}[0],
			},
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "invalid amount in event data",
		},
		{
			name:          "V4 Invalid amount data type",
			txMetaVersion: 4,
			eventType:     EventTypeTransfer,
			topics: []xdr.ScVal{
				makeSymbol("transfer"),
				makeAddress(randomAccount),
				makeAddress(zeroContract),
				makeAsset(randomAsset),
			},
			data: xdr.ScVal{
				Type: xdr.ScValTypeScvU64, // Should be i128
				U64:  &[]xdr.Uint64{1000}[0],
			},
			asset:         randomAsset,
			contractID:    mustGetContractID(randomAsset),
			expectedError: "invalid amount in event data",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create the contract event
			event := &xdr.ContractEvent{
				Type:       xdr.ContractEventTypeContract,
				ContractId: tc.contractID,
				Body: xdr.ContractEventBody{
					V: 0,
					V0: &xdr.ContractEventV0{
						Topics: tc.topics,
						Data:   tc.data,
					},
				},
			}

			// Create the transaction
			tx := someLedgerTransaction(tc.txMetaVersion)

			// Parse the event
			result, err := NewStellarAssetContractEvent(tx, event, passphrase)

			if tc.expectedError != "" {
				// Expecting an error
				require.Error(t, err, "Expected error for test case: %s", tc.name)
				assert.Contains(t, err.Error(), tc.expectedError, "Error message mismatch for: %s", tc.name)
				assert.Nil(t, result, "Result should be nil when error expected for: %s", tc.name)
			} else {
				// Expecting success
				require.NoError(t, err, "Unexpected error for test case: %s", tc.name)
				require.NotNil(t, result, "Result should not be nil for: %s", tc.name)

				// Compare the results
				assert.Equal(t, tc.expectedResult.Type, result.Type, "Event type mismatch for: %s", tc.name)
				assert.Equal(t, tc.expectedResult.Asset, result.Asset, "asset mismatch for: %s", tc.name)
				assert.Equal(t, tc.expectedResult.From, result.From, "From address mismatch for: %s", tc.name)
				assert.Equal(t, tc.expectedResult.To, result.To, "To address mismatch for: %s", tc.name)
				assert.Equal(t, tc.expectedResult.Amount, result.Amount, "Amount mismatch for: %s", tc.name)
				assert.Equal(t, tc.expectedResult.DestinationMemo, result.DestinationMemo, "Memo mismatch for: %s", tc.name)
			}
		})
	}
}

// Test helper functions
func someLedgerTransaction(version int32) ingest.LedgerTransaction {
	return ingest.LedgerTransaction{
		UnsafeMeta: xdr.TransactionMeta{
			V: version,
		},
	}
}

func mustGetContractID(asset xdr.Asset) *xdr.ContractId {
	id, err := asset.ContractID(passphrase)
	if err != nil {
		panic(err)
	}
	contractId := xdr.ContractId(id)
	return &contractId
}
