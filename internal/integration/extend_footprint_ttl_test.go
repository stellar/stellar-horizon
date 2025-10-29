package integration

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/protocols/horizon/operations"
	"github.com/stellar/go/services/horizon/internal/test/integration"
)

func TestExtendFootprintTtl(t *testing.T) {
	if integration.GetCoreMaxSupportedProtocol() < 20 {
		t.Skip("This test run does not support less than Protocol 20")
	}

	itest := integration.NewTest(t, integration.Config{
		EnableStellarRPC: true,
		QuickExpiration:  true,
	})

	// establish which account will be contract owner, and load it's current seq
	sourceAccount, err := itest.Client().AccountDetail(horizonclient.AccountRequest{
		AccountID: itest.Master().Address(),
	})
	require.NoError(t, err)

	installContractOp := assembleInstallContractCodeOp(t, itest.Master().Address(), add_u64_contract)
	preFlightOp := itest.PreflightHostFunctions(&sourceAccount, *installContractOp)
	tx := itest.MustSubmitOperations(&sourceAccount, itest.Master(), &preFlightOp)

	_, err = itest.Client().TransactionDetail(tx.Hash)
	require.NoError(t, err)

	sourceAccount, bumpFootPrint := itest.PreflightExtendExpiration(
		itest.Master().Address(),
		preFlightOp.Ext.SorobanData.Resources.Footprint.ReadWrite,
		10000,
	)
	tx = itest.MustSubmitOperations(&sourceAccount, itest.Master(), &bumpFootPrint)

	ops, err := itest.Client().Operations(horizonclient.OperationRequest{ForTransaction: tx.Hash})
	require.NoError(t, err)
	require.Len(t, ops.Embedded.Records, 1)

	op := ops.Embedded.Records[0].(operations.ExtendFootprintTtl)
	require.Equal(t, uint32(10000), op.ExtendTo)
}
