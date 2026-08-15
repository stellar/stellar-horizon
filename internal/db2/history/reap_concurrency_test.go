package history

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/stellar-horizon/internal/test"
)

// TestReapDoesNotDeleteConcurrentlyReferencedRows is a regression test for a bug
// where the lookup table reaper deleted a history_accounts row that live
// ingestion was concurrently inserting a reference to.
//
// The reaper selects a candidate row while it is still orphaned, then, in a
// separate transaction, re-checks that it is orphaned before deleting it. Live
// ingestion locks the row FOR KEY SHARE and inserts a referencing child row in a
// single transaction. The reaper's DELETE blocks on that lock, but under READ
// COMMITTED blocking alone does not refresh the snapshot used by the orphan
// check: if the lock and the NOT EXISTS check share one statement, the check
// runs against the pre-commit snapshot, misses the new reference, and deletes the
// row while it is in fact referenced. This test drives exactly that interleaving
// and asserts the reaper leaves the row in place.
func TestReapDoesNotDeleteConcurrentlyReferencedRows(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	ctx := context.Background()
	s1 := tt.HorizonSession()
	s2 := s1.Clone()

	// Create a bare account row (no references), which makes it a reap candidate.
	address := keypair.MustRandom().Address()
	setup := NewAccountLoader(ConcurrentDeletes)
	setup.GetFuture(address)
	assert.NoError(t, setup.Exec(ctx, s1))
	accountID, err := setup.GetNow(address)
	assert.NoError(t, err)

	// Ingestion transaction: lock the account row FOR KEY SHARE, exactly as the
	// loader does during live ingestion, then insert an operation participant
	// referencing it. Do not commit yet, so the reference is still uncommitted
	// while the reaper runs.
	assert.NoError(t, s1.Begin(ctx))

	ingestLoader := NewAccountLoader(ConcurrentDeletes)
	future := ingestLoader.GetFuture(address)
	assert.NoError(t, ingestLoader.Exec(ctx, s1))

	q1 := &Q{s1}
	opParticipants := q1.NewOperationParticipantBatchInsertBuilder()
	assert.NoError(t, opParticipants.Add(int64(1), future))
	assert.NoError(t, opParticipants.Exec(ctx, s1))

	// Reaper transaction, running concurrently with the ingestion transaction.
	assert.NoError(t, s2.Begin(ctx))
	q2 := &Q{s2}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Give the reaper time to start and block on the FOR KEY SHARE lock held
		// by the ingestion transaction before we commit that transaction.
		<-time.After(time.Second * 3)
		assert.NoError(t, s1.Commit())
	}()

	// reapLookupTable blocks on the ingestion transaction's row lock. Once that
	// transaction commits, the DELETE runs under a fresh snapshot and must observe
	// the operation participant reference, so nothing is deleted.
	deletedCount, err := q2.reapLookupTable(ctx, "history_accounts", []int64{accountID}, 1000)
	assert.NoError(t, err)
	assert.NoError(t, s2.Commit())
	wg.Wait()

	assert.Equal(t, int64(0), deletedCount)

	// The account row must still exist and keep its original id.
	var account Account
	assert.NoError(t, q2.AccountByAddress(ctx, &account, address))
	assert.Equal(t, accountID, account.ID)
}
