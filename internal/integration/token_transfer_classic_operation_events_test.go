package integration

import (
	"fmt"
	assetProto "github.com/stellar/go/asset"
	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/processors/token_transfer"
	"github.com/stellar/go/services/horizon/internal/test/integration"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"testing"
	"time"
)

var (
	revokeTrustline = func(issuer, trustor string, asset txnbuild.Asset) *txnbuild.SetTrustLineFlags {
		return &txnbuild.SetTrustLineFlags{
			SourceAccount: issuer,
			Trustor:       trustor,
			Asset:         asset,
			ClearFlags:    []txnbuild.TrustLineFlag{txnbuild.TrustLineAuthorized},
		}
	}
	// Give the master account the revocable flag (needed to set the clawback flag)
	setRevocableFlag = txnbuild.SetOptions{
		SetFlags: []txnbuild.AccountFlag{
			txnbuild.AuthRevocable,
		},
	}

	addTrustlineForAssetOp = func(forAccount string, asset txnbuild.Asset) *txnbuild.ChangeTrust {
		return &txnbuild.ChangeTrust{
			Line: txnbuild.ChangeTrustAssetWrapper{
				Asset: asset,
			},
			Limit:         txnbuild.MaxTrustlineLimit,
			SourceAccount: forAccount,
		}

	}

	addTrustlineForLiquidityPoolOp = func(forAccount string, assetA txnbuild.Asset, assetB txnbuild.Asset) *txnbuild.ChangeTrust {
		return &txnbuild.ChangeTrust{
			SourceAccount: forAccount,
			Line: txnbuild.LiquidityPoolShareChangeTrustAsset{
				LiquidityPoolParameters: txnbuild.LiquidityPoolParameters{
					AssetA: assetA,
					AssetB: assetB,
					Fee:    30,
				},
			},
			Limit: txnbuild.MaxTrustlineLimit,
		}
	}

	paymentOp = func(src string, dest string, asset txnbuild.Asset, amount string) *txnbuild.Payment {
		return &txnbuild.Payment{
			SourceAccount: src,
			Destination:   dest,
			Asset:         asset,
			Amount:        amount,
		}
	}
)

func assertFeeEvent(t *testing.T, events []*token_transfer.TokenTransferEvent, from string, amt string) {
	require.Condition(t, func() bool {
		for _, event := range events {
			if event.GetEventType() == token_transfer.FeeEvent &&
				event.GetFee().From == from &&
				event.GetFee().Amount == amt {
				return true
			}
		}
		return false
	}, "Expected a fee event with amount %s, but not found", amt)
}

func assertTransferEvent(t *testing.T, events []*token_transfer.TokenTransferEvent, from, to string, asset *assetProto.Asset, amt string) {
	require.Condition(t, func() bool {
		for _, event := range events {
			if event.GetEventType() == token_transfer.TransferEvent &&
				event.GetTransfer().From == from &&
				event.GetTransfer().To == to &&
				event.GetTransfer().Amount == amt &&
				event.GetAsset().Equals(asset) {
				return true
			}
		}
		return false
	}, "Expected transfer event: %s -> %s, amount: %s, asset: %s, but not found", from, to, amt, asset)
}

// Assert that a mint event exists
func assertMintEvent(t *testing.T, events []*token_transfer.TokenTransferEvent, to string, amt string, asset *assetProto.Asset) {
	require.Condition(t, func() bool {
		for _, event := range events {
			if event.GetEventType() == token_transfer.MintEvent &&
				event.GetMint().To == to &&
				event.GetAsset().Equals(asset) &&
				event.GetMint().Amount == amt {
				return true
			}
		}
		return false
	}, "Expected mint event: to %s, asset: %s, amount: %s, but not found", to, asset, amt)
}

// Assert that a burn event exists
func assertBurnEvent(t *testing.T, events []*token_transfer.TokenTransferEvent, from string, amt string, asset *assetProto.Asset) {
	require.Condition(t, func() bool {
		for _, event := range events {
			if event.GetEventType() == token_transfer.BurnEvent &&
				event.GetBurn().From == from &&
				event.GetAsset().Equals(asset) &&
				event.GetBurn().Amount == amt {
				return true
			}
		}
		return false
	}, "Expected burn event: from %s, asset: %s, amount: %s, but not found", from, asset, amt)
}

// Retaining this function to help with debugging
func printProtoEvents(events []*token_transfer.TokenTransferEvent) {
	for _, event := range events {
		jsonBytes, _ := protojson.MarshalOptions{
			Multiline:         true,
			EmitDefaultValues: true,
			Indent:            "  ",
		}.Marshal(event)
		fmt.Printf("### Event Type : %v\n", event.GetEventType())
		fmt.Println(string(jsonBytes))
		fmt.Println("###")
	}
}

func TestTrustlineRevocationEvents(t *testing.T) {
	tt := assert.New(t)
	itest := integration.NewTest(t, integration.Config{})
	master := itest.Master()

	//Setup
	keys, accounts := itest.CreateAccounts(2, "1000000000")
	// One simple account that participates in LP-USDC-XLM
	lpParticipantAccountKeys, lpParticipantAccount := keys[0], accounts[0]
	// Eth Issuer account that participates in LP-USDC-ETH
	ethAccountKeys, ethAccount := keys[1], accounts[1]

	itest.MustSubmitOperations(itest.MasterAccount(), master, &setRevocableFlag)
	itest.MustSubmitOperations(ethAccount, ethAccountKeys, &setRevocableFlag)

	usdcAsset := txnbuild.CreditAsset{
		Code:   "USDC",
		Issuer: master.Address(),
	}

	ethAsset := txnbuild.CreditAsset{
		Code:   "ETH",
		Issuer: ethAccount.GetAccountID(),
	}
	xlmAsset := txnbuild.NativeAsset{}

	itest.MustSubmitMultiSigOperations(itest.MasterAccount(),
		[]*keypair.Full{lpParticipantAccountKeys, master, ethAccountKeys},

		addTrustlineForAssetOp(lpParticipantAccount.GetAccountID(), usdcAsset),
		addTrustlineForAssetOp(ethAccount.GetAccountID(), usdcAsset),
		addTrustlineForAssetOp(master.Address(), ethAsset),

		addTrustlineForLiquidityPoolOp(lpParticipantAccount.GetAccountID(), xlmAsset, usdcAsset),
		addTrustlineForLiquidityPoolOp(ethAccount.GetAccountID(), ethAsset, usdcAsset),

		paymentOp(master.Address(), lpParticipantAccount.GetAccountID(), usdcAsset, "1000"),
		paymentOp(master.Address(), ethAccount.GetAccountID(), usdcAsset, "1000"),
	)

	usdcXlmPoolId, _ := xdr.NewPoolId(
		xdr.MustNewNativeAsset(),
		xdr.MustNewCreditAsset(usdcAsset.Code, usdcAsset.Issuer),
		30,
	)

	usdcEthPoolId, _ := xdr.NewPoolId(
		xdr.MustNewCreditAsset(ethAsset.Code, ethAsset.Issuer),
		xdr.MustNewCreditAsset(usdcAsset.Code, usdcAsset.Issuer),
		30,
	)

	usdcXlmPoolIDHexString := xdr.Hash(usdcXlmPoolId).HexString()
	usdcEthPoolIdHexString := xdr.Hash(usdcEthPoolId).HexString()

	itest.MustSubmitMultiSigOperations(itest.MasterAccount(),
		[]*keypair.Full{master, lpParticipantAccountKeys, ethAccountKeys},
		&txnbuild.LiquidityPoolDeposit{
			SourceAccount:   lpParticipantAccount.GetAccountID(),
			LiquidityPoolID: [32]byte(usdcXlmPoolId),
			MaxAmountA:      "400",
			MaxAmountB:      "777",
			MinPrice:        xdr.Price{N: 1, D: 2},
			MaxPrice:        xdr.Price{N: 2, D: 1},
		},
		&txnbuild.LiquidityPoolDeposit{
			SourceAccount:   ethAccount.GetAccountID(),
			LiquidityPoolID: [32]byte(usdcEthPoolId),
			MaxAmountA:      "400",
			MaxAmountB:      "777",
			MinPrice:        xdr.Price{N: 1, D: 2},
			MaxPrice:        xdr.Price{N: 2, D: 1},
		},
	)

	// Some sanity asserts
	_, err := itest.Client().LiquidityPoolDetail(horizonclient.LiquidityPoolRequest{
		LiquidityPoolID: usdcXlmPoolIDHexString,
	})
	tt.NoError(err)
	_, err = itest.Client().LiquidityPoolDetail(horizonclient.LiquidityPoolRequest{
		LiquidityPoolID: usdcEthPoolIdHexString,
	})
	tt.NoError(err)

	// Actual test fixture
	revokeTrustlineTxResp := itest.MustSubmitMultiSigOperations(
		itest.MasterAccount(),
		[]*keypair.Full{master},
		// This operation shud generate 2 transfer events
		revokeTrustline(master.Address(), lpParticipantAccount.GetAccountID(), usdcAsset),
		// Revoking USDC trustline for EthIssuer.. BURN
		revokeTrustline(master.Address(), ethAccount.GetAccountID(), usdcAsset),
	)

	ledgerSeq := uint32(revokeTrustlineTxResp.Ledger)
	itest.WaitForLedgerInArchive(30*time.Second, ledgerSeq)
	ledger := getLedgers(itest, ledgerSeq, ledgerSeq)[ledgerSeq]

	ttp := token_transfer.NewEventsProcessor(itest.GetPassPhrase())
	events, err := ttp.EventsFromLedger(ledger)
	tt.NoError(err)

	t = itest.CurrentTest()
	//printProtoEvents(events)

	// 2 operations - 100 stroops per operation
	assertFeeEvent(t, events, master.Address(), "200")
	assert.NoError(t, token_transfer.VerifyEvents(ledger, itest.GetPassPhrase(), false))

	// TODO - Add assertions for transfer with CB and LP, once Strkey support is added
}
