package history

import (
	"database/sql"
	"testing"

	"github.com/stellar/go/services/horizon/internal/test"
)

func TestGetLoadTestRestoreStateEmptyDB(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &Q{tt.HorizonSession()}

	runID, ledger, err := q.GetLoadTestRestoreState(tt.Ctx)
	tt.Require.Equal("", runID)
	tt.Require.Equal(uint32(0), ledger)
	tt.Require.ErrorIs(err, sql.ErrNoRows)
}

func TestGetLoadTestRestoreState_Inconsistent_OnlyLedger(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &Q{tt.HorizonSession()}

	// Insert only the ledger key
	err := q.updateValueInStore(tt.Ctx, loadTestLedgerKey, "123")
	tt.Require.NoError(err)

	_, _, err = q.GetLoadTestRestoreState(tt.Ctx)
	tt.Require.ErrorContains(err, "load test restore state is inconsistent")
}

func TestGetLoadTestRestoreState_Inconsistent_OnlyRunID(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &Q{tt.HorizonSession()}

	// Insert only the runID key
	err := q.updateValueInStore(tt.Ctx, loadTestRunID, "run-1")
	tt.Require.NoError(err)

	_, _, err = q.GetLoadTestRestoreState(tt.Ctx)
	tt.Require.ErrorContains(err, "load test restore state is inconsistent")
}

func TestSetAndGetLoadTestRestoreState(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &Q{tt.HorizonSession()}

	err := q.SetLoadTestRestoreState(tt.Ctx, "run-abc", 456)
	tt.Require.NoError(err)

	runID, ledger, err := q.GetLoadTestRestoreState(tt.Ctx)
	tt.Require.NoError(err)
	tt.Require.Equal("run-abc", runID)
	tt.Require.Equal(uint32(456), ledger)
}

func TestClearLoadTestRestoreState(t *testing.T) {
	tt := test.Start(t)
	defer tt.Finish()
	test.ResetHorizonDB(t, tt.HorizonDB)

	q := &Q{tt.HorizonSession()}

	// Set initial state
	err := q.SetLoadTestRestoreState(tt.Ctx, "run-abc", 789)
	tt.Require.NoError(err)

	// Clear state
	err = q.ClearLoadTestRestoreState(tt.Ctx)
	tt.Require.NoError(err)

	// Getting should now return sql.ErrNoRows
	_, _, err = q.GetLoadTestRestoreState(tt.Ctx)
	tt.Require.ErrorIs(err, sql.ErrNoRows)

	// Clearing again on empty state should not error
	err = q.ClearLoadTestRestoreState(tt.Ctx)
	tt.Require.NoError(err)
}
