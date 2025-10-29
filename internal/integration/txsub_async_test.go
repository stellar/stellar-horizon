package integration

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go/clients/horizonclient"
	"github.com/stellar/go/keypair"
	"github.com/stellar/go/protocols/horizon"
	"github.com/stellar/go/services/horizon/internal/test/integration"
	"github.com/stellar/go/support/errors"
	"github.com/stellar/go/txnbuild"
)

func getTransaction(client *horizonclient.Client, hash string) error {
	for i := 0; i < 60; i++ {
		_, err := client.TransactionDetail(hash)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}

		return nil
	}
	return errors.New("transaction not found")
}

func TestAsyncTxSub_SuccessfulSubmission(t *testing.T) {
	itest := integration.NewTest(t, integration.Config{})
	master := itest.Master()
	masterAccount := itest.MasterAccount()

	txParams := txnbuild.TransactionParams{
		BaseFee:              txnbuild.MinBaseFee,
		SourceAccount:        masterAccount,
		IncrementSequenceNum: true,
		Operations: []txnbuild.Operation{
			&txnbuild.Payment{
				Destination: master.Address(),
				Amount:      "10",
				Asset:       txnbuild.NativeAsset{},
			},
		},
		Preconditions: txnbuild.Preconditions{
			TimeBounds:   txnbuild.NewInfiniteTimeout(),
			LedgerBounds: &txnbuild.LedgerBounds{MinLedger: 0, MaxLedger: 100},
		},
	}
	tx, err := itest.CreateSignedTransaction([]*keypair.Full{master}, txParams)
	assert.NoError(t, err)
	expectedHash, err := tx.HashHex(itest.GetPassPhrase())
	assert.NoError(t, err)

	txResp, err := itest.Client().AsyncSubmitTransaction(tx)
	assert.NoError(t, err)
	assert.Equal(t, txResp, horizon.AsyncTransactionSubmissionResponse{
		TxStatus: "PENDING",
		Hash:     expectedHash,
	})

	err = getTransaction(itest.Client(), txResp.Hash)
	assert.NoError(t, err)
}

func TestAsyncTxSub_SubmissionError(t *testing.T) {
	itest := integration.NewTest(t, integration.Config{})
	master := itest.Master()
	masterAccount := itest.MasterAccount()

	txParams := txnbuild.TransactionParams{
		BaseFee:              txnbuild.MinBaseFee,
		SourceAccount:        masterAccount,
		IncrementSequenceNum: false,
		Operations: []txnbuild.Operation{
			&txnbuild.Payment{
				Destination: master.Address(),
				Amount:      "10",
				Asset:       txnbuild.NativeAsset{},
			},
		},
		Preconditions: txnbuild.Preconditions{
			TimeBounds:   txnbuild.NewInfiniteTimeout(),
			LedgerBounds: &txnbuild.LedgerBounds{MinLedger: 0, MaxLedger: 100},
		},
	}

	tx, err := itest.CreateSignedTransaction([]*keypair.Full{master}, txParams)
	assert.NoError(t, err)
	expectedHash, err := tx.HashHex(itest.GetPassPhrase())
	assert.NoError(t, err)

	txResp, err := itest.Client().AsyncSubmitTransaction(tx)
	assert.NoError(t, err)
	assert.Equal(t, txResp, horizon.AsyncTransactionSubmissionResponse{
		ErrorResultXDR: "AAAAAAAAAGT////7AAAAAA==",
		TxStatus:       "ERROR",
		Hash:           expectedHash,
	})
}

func TestAsyncTxSub_SubmissionTryAgainLater(t *testing.T) {
	itest := integration.NewTest(t, integration.Config{})
	master := itest.Master()
	masterAccount := itest.MasterAccount()

	txParams := txnbuild.TransactionParams{
		BaseFee:              txnbuild.MinBaseFee,
		SourceAccount:        masterAccount,
		IncrementSequenceNum: true,
		Operations: []txnbuild.Operation{
			&txnbuild.Payment{
				Destination: master.Address(),
				Amount:      "10",
				Asset:       txnbuild.NativeAsset{},
			},
		},
		Preconditions: txnbuild.Preconditions{
			TimeBounds:   txnbuild.NewInfiniteTimeout(),
			LedgerBounds: &txnbuild.LedgerBounds{MinLedger: 0, MaxLedger: 100},
		},
	}

	tx, err := itest.CreateSignedTransaction([]*keypair.Full{master}, txParams)
	assert.NoError(t, err)
	expectedHash, err := tx.HashHex(itest.GetPassPhrase())
	assert.NoError(t, err)

	txResp, err := itest.Client().AsyncSubmitTransaction(tx)
	assert.NoError(t, err)
	assert.Equal(t, txResp, horizon.AsyncTransactionSubmissionResponse{
		ErrorResultXDR: "",
		TxStatus:       "PENDING",
		Hash:           expectedHash,
	})

	tx, err = itest.CreateSignedTransaction([]*keypair.Full{master}, txParams)
	assert.NoError(t, err)
	expectedHash, err = tx.HashHex(itest.GetPassPhrase())
	assert.NoError(t, err)

	txResp, err = itest.Client().AsyncSubmitTransaction(tx)
	assert.NoError(t, err)
	assert.Equal(t, txResp, horizon.AsyncTransactionSubmissionResponse{
		ErrorResultXDR: "",
		TxStatus:       "TRY_AGAIN_LATER",
		Hash:           expectedHash,
	})
}

func TestAsyncTxSub_TransactionMalformed(t *testing.T) {
	itest := integration.NewTest(t, integration.Config{
		EnableStellarRPC: true,
		HorizonEnvironment: map[string]string{
			"MAX_HTTP_REQUEST_SIZE": "1800",
		},
		QuickExpiration: true,
	})
	master := itest.Master()

	// establish which account will be contract owner, and load it's current seq
	sourceAccount, err := itest.Client().AccountDetail(horizonclient.AccountRequest{
		AccountID: itest.Master().Address(),
	})
	require.NoError(t, err)

	installContractOp := assembleInstallContractCodeOp(t, master.Address(), "soroban_sac_test.wasm")
	preFlightOp := itest.PreflightHostFunctions(&sourceAccount, *installContractOp)
	txParams := txnbuild.TransactionParams{
		SourceAccount:        &sourceAccount,
		Operations:           []txnbuild.Operation{&preFlightOp},
		BaseFee:              txnbuild.MinBaseFee,
		Preconditions:        txnbuild.Preconditions{TimeBounds: txnbuild.NewInfiniteTimeout()},
		IncrementSequenceNum: true,
	}
	_, err = itest.AsyncSubmitTransaction(master, txParams)
	assert.EqualError(
		t, err,
		"horizon error: \"Transaction Malformed\" - check horizon.Error.Problem for more information",
	)
}

func TestAsyncTxSub_GetOpenAPISpecResponse(t *testing.T) {
	itest := integration.NewTest(t, integration.Config{})
	res, err := http.Get(itest.AsyncTxSubOpenAPISpecURL())
	assert.NoError(t, err)
	assert.Equal(t, res.StatusCode, 200)

	bytes, err := io.ReadAll(res.Body)
	res.Body.Close()
	assert.NoError(t, err)

	openAPISpec := string(bytes)
	assert.Contains(t, openAPISpec, "openapi: 3.0.0")
}
