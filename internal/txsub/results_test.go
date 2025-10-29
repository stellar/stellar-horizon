package txsub

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/services/horizon/internal/test"
	"github.com/stretchr/testify/mock"
)

func TestGetIngestedTx(t *testing.T) {
	tt := test.Start(t)
	tt.Scenario("base")
	defer tt.Finish()
	q := &history.Q{SessionInterface: tt.HorizonSession()}
	hash := "2374e99349b9ef7dba9a5db3339b78fda8f34777b1af33ba468ad5c0df946d4d"
	tx, err := txResultByHash(tt.Ctx, q, hash)
	tt.Assert.NoError(err)
	tt.Assert.Equal(hash, tx.TransactionHash)
}

func TestGetIngestedTxHashes(t *testing.T) {
	tt := test.Start(t)
	tt.Scenario("base")
	defer tt.Finish()
	q := &history.Q{SessionInterface: tt.HorizonSession()}
	hashes := []string{"2374e99349b9ef7dba9a5db3339b78fda8f34777b1af33ba468ad5c0df946d4d"}
	txs, err := q.AllTransactionsByHashesSinceLedger(tt.Ctx, hashes, 0)
	tt.Assert.NoError(err)
	tt.Assert.Equal(hashes[0], txs[0].TransactionHash)
}

func TestGetMissingTx(t *testing.T) {
	tt := test.Start(t)
	tt.Scenario("base")
	defer tt.Finish()
	q := &history.Q{SessionInterface: tt.HorizonSession()}
	hash := "adf1efb9fd253f53cbbe6230c131d2af19830328e52b610464652d67d2fb7195"

	_, err := txResultByHash(tt.Ctx, q, hash)
	tt.Assert.Equal(ErrNoResults, err)
}

func TestGetFailedTx(t *testing.T) {
	tt := test.Start(t)
	tt.Scenario("failed_transactions")
	defer tt.Finish()
	q := &history.Q{SessionInterface: tt.HorizonSession()}
	hash := "aa168f12124b7c196c0adaee7c73a64d37f99428cacb59a91ff389626845e7cf"

	_, err := txResultByHash(tt.Ctx, q, hash)
	tt.Assert.Equal("AAAAAAAAAGT/////AAAAAQAAAAAAAAAB/////gAAAAA=", err.(*FailedTransactionError).ResultXDR)
}

func TestFilteredQueryErrs(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()

	q := &mockDBQ{}
	hash := "aa168f12124b7c196c0adaee7c73a64d37f99428cacb59a91ff389626845e7cf"

	q.On("PreFilteredTransactionByHash", tt.Ctx, mock.Anything, hash).Return(sql.ErrConnDone).Once()
	q.On("NoRows", sql.ErrConnDone).Return(false).Once()
	_, err := txResultByHash(tt.Ctx, q, hash)
	tt.Assert.True(errors.Is(err, sql.ErrConnDone))
}

func TestHistoryQueryErrs(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()

	q := &mockDBQ{}
	hash := "aa168f12124b7c196c0adaee7c73a64d37f99428cacb59a91ff389626845e7cf"

	q.On("PreFilteredTransactionByHash", tt.Ctx, mock.Anything, hash).Return(sql.ErrNoRows).Once()
	q.On("NoRows", sql.ErrNoRows).Return(true).Once()

	q.On("TransactionByHash", tt.Ctx, mock.Anything, hash).Return(sql.ErrConnDone).Once()
	q.On("NoRows", sql.ErrConnDone).Return(false).Once()

	_, err := txResultByHash(tt.Ctx, q, hash)
	tt.Assert.True(errors.Is(err, sql.ErrConnDone))
}
