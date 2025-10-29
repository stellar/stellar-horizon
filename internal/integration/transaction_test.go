package integration

import (
	"testing"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/services/horizon/internal/test/integration"
	"github.com/stellar/go/txnbuild"
	"github.com/stellar/go/xdr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestP19MetaTransaction(t *testing.T) {
	itest := integration.NewTest(t, integration.Config{
		ProtocolVersion:  19,
		EnableStellarRPC: false,
	})

	masterAccount, err := itest.Client().AccountDetail(horizonclient.AccountRequest{
		AccountID: itest.Master().Address(),
	})
	require.NoError(t, err)

	op := &txnbuild.Payment{
		SourceAccount: itest.Master().Address(),
		Destination:   itest.Master().Address(),
		Asset:         txnbuild.NativeAsset{},
		Amount:        "10",
	}

	clientTx := itest.MustSubmitOperations(&masterAccount, itest.Master(), op)

	var txMetaResult xdr.TransactionMeta
	err = xdr.SafeUnmarshalBase64(clientTx.ResultMetaXdr, &txMetaResult)
	require.NoError(t, err)

	assert.Greater(t, len(txMetaResult.MustV2().Operations), 0)
	assert.Greater(t, len(txMetaResult.MustV2().TxChangesBefore), 0)
	// TODO figure out how to generate TxChangesAfter also
	//assert.Greater(t, len(txMetaResult.MustV2().TxChangesAfter), 0)
}

func TestP19MetaDisabledTransaction(t *testing.T) {
	itest := integration.NewTest(t, integration.Config{
		ProtocolVersion:    19,
		HorizonEnvironment: map[string]string{"SKIP_TXMETA": "TRUE"},
		EnableStellarRPC:   false,
	})

	masterAccount, err := itest.Client().AccountDetail(horizonclient.AccountRequest{
		AccountID: itest.Master().Address(),
	})
	require.NoError(t, err)

	op := &txnbuild.Payment{
		SourceAccount: itest.Master().Address(),
		Destination:   itest.Master().Address(),
		Asset:         txnbuild.NativeAsset{},
		Amount:        "10",
	}

	clientTx := itest.MustSubmitOperations(&masterAccount, itest.Master(), op)

	assert.Empty(t, clientTx.ResultMetaXdr)
}

func TestP20MetaTransaction(t *testing.T) {
	if integration.GetCoreMaxSupportedProtocol() < 20 {
		t.Skip("This test run does not support less than Protocol 20")
	}

	itest := integration.NewTest(t, integration.Config{
		EnableStellarRPC: true,
	})

	// establish which account will be contract owner, and load it's current seq
	sourceAccount, err := itest.Client().AccountDetail(horizonclient.AccountRequest{
		AccountID: itest.Master().Address(),
	})
	require.NoError(t, err)

	installContractOp := assembleInstallContractCodeOp(t, itest.Master().Address(), add_u64_contract)
	preFlightOp := itest.PreflightHostFunctions(&sourceAccount, *installContractOp)
	clientTx := itest.MustSubmitOperations(&sourceAccount, itest.Master(), &preFlightOp)

	var txMetaResult xdr.TransactionMeta
	err = xdr.SafeUnmarshalBase64(clientTx.ResultMetaXdr, &txMetaResult)
	require.NoError(t, err)

	switch txMetaResult.V {
	case 3:
		assert.Greater(t, len(txMetaResult.MustV3().Operations), 0)
		assert.NotNil(t, txMetaResult.MustV3().SorobanMeta)
		assert.Greater(t, len(txMetaResult.MustV3().TxChangesAfter), 0)
		assert.Greater(t, len(txMetaResult.MustV3().TxChangesBefore), 0)
	case 4:
		assert.Greater(t, len(txMetaResult.MustV4().Operations), 0)
		assert.NotNil(t, txMetaResult.MustV4().SorobanMeta)
		// Soroban fee refund was moved from txChangesAfter to postTxApplyFeeProcessing in LedgerCloseMetaV2
		// see https://github.com/stellar/stellar-protocol/blob/master/core/cap-0063.md
		assert.Greater(t, len(txMetaResult.MustV4().TxChangesBefore), 0)
	default:
		itest.CurrentTest().Fatalf("Invalid meta version: %d", txMetaResult.V)
	}

}

func TestP20MetaDisabledTransaction(t *testing.T) {
	if integration.GetCoreMaxSupportedProtocol() < 20 {
		t.Skip("This test run does not support less than Protocol 20")
	}

	itest := integration.NewTest(t, integration.Config{
		HorizonEnvironment: map[string]string{"SKIP_TXMETA": "TRUE"},
		EnableStellarRPC:   true,
	})

	// establish which account will be contract owner, and load it's current seq
	sourceAccount, err := itest.Client().AccountDetail(horizonclient.AccountRequest{
		AccountID: itest.Master().Address(),
	})
	require.NoError(t, err)

	installContractOp := assembleInstallContractCodeOp(t, itest.Master().Address(), add_u64_contract)
	preFlightOp := itest.PreflightHostFunctions(&sourceAccount, *installContractOp)
	clientTx := itest.MustSubmitOperations(&sourceAccount, itest.Master(), &preFlightOp)

	assert.Empty(t, clientTx.ResultMetaXdr)
}
