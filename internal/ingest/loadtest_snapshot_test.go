package ingest

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/stellar/go/toid"
)

func TestLoadTestSaveSnapshot(t *testing.T) {
	ctx := context.Background()
	q := &mockDBQ{}

	q.On("Begin", ctx).Return(nil).Once()
	q.On("GetLoadTestRestoreState", ctx).Return("", uint32(0), sql.ErrNoRows).Once()
	q.On("GetLastLedgerIngest", ctx).Return(uint32(123), nil).Once()
	q.On("SetLoadTestRestoreState", ctx, mock.AnythingOfType("string"), uint32(123)).Return(nil).Once()
	q.On("Commit").Return(nil).Once()
	q.On("Rollback").Return(nil).Once()

	l := &loadTestSnapshot{HistoryQ: q}
	require.NoError(t, l.save(ctx))
	require.NotEmpty(t, l.runId)

	q.AssertExpectations(t)
}

func TestLoadTestSaveSnapshotAlreadyActiveLocal(t *testing.T) {
	ctx := context.Background()
	q := &mockDBQ{}

	// Begin then immediately error due to local run ID, then rollback
	q.On("Begin", ctx).Return(nil).Once()
	q.On("Rollback").Return(nil).Once()

	l := &loadTestSnapshot{HistoryQ: q, runId: "existing"}
	require.ErrorContains(t, l.save(ctx), "already active")

	q.AssertExpectations(t)
}

func TestLoadTestSaveSnapshotAlreadyActiveRemote(t *testing.T) {
	ctx := context.Background()
	q := &mockDBQ{}

	q.On("Begin", ctx).Return(nil).Once()
	q.On("GetLastLedgerIngest", ctx).Return(uint32(999), nil).Once()
	q.On("GetLoadTestRestoreState", ctx).Return("rid", uint32(150), nil).Once()
	q.On("Rollback").Return(nil).Once()

	l := &loadTestSnapshot{HistoryQ: q}
	require.ErrorContains(t, l.save(ctx), "already active")
	require.Empty(t, l.runId)

	q.AssertExpectations(t)
}

func TestLoadTestRestoreNoop(t *testing.T) {
	ctx := context.Background()
	q := &mockDBQ{}

	q.On("Begin", ctx).Return(nil).Once()
	q.On("GetLastLedgerIngest", ctx).Return(uint32(321), nil).Once()
	q.On("GetLoadTestRestoreState", ctx).Return("", uint32(0), sql.ErrNoRows).Once()
	q.On("Rollback").Return(nil).Once()

	require.NoError(t, RestoreSnapshot(ctx, q))

	q.AssertExpectations(t)
}

func TestLoadTestRestore(t *testing.T) {
	ctx := context.Background()
	q := &mockDBQ{}

	last := uint32(200)
	restore := uint32(150)

	q.On("Begin", ctx).Return(nil).Once()
	q.On("GetLastLedgerIngest", ctx).Return(last, nil).Once()
	q.On("GetLoadTestRestoreState", ctx).Return("rid", restore, nil).Once()

	var capturedStart int64
	var capturedEnd int64
	q.On("DeleteRangeAll", ctx, mock.AnythingOfType("int64"), mock.AnythingOfType("int64")).Return(int64(0), nil).Run(func(args mock.Arguments) {
		capturedStart = args.Get(1).(int64)
		capturedEnd = args.Get(2).(int64)
	}).Once()

	q.On("ClearLoadTestRestoreState", ctx).Return(nil).Once()
	q.On("UpdateIngestVersion", ctx, 0).Return(nil).Once()
	q.On("Commit").Return(nil).Once()
	q.On("Rollback").Return(nil).Once()

	require.NoError(t, RestoreSnapshot(ctx, q))

	expectedStart, expectedEnd, err := toid.LedgerRangeInclusive(int32(restore+1), int32(last))
	require.NoError(t, err)
	require.Equal(t, expectedStart, capturedStart)
	require.Equal(t, expectedEnd, capturedEnd)

	q.AssertExpectations(t)
}

func TestLoadTestRestoreInvalidLastLedger(t *testing.T) {
	ctx := context.Background()
	q := &mockDBQ{}

	q.On("Begin", ctx).Return(nil).Once()
	q.On("GetLastLedgerIngest", ctx).Return(uint32(100), nil).Once()
	q.On("GetLoadTestRestoreState", ctx).Return("rid", uint32(150), nil).Once()
	q.On("Rollback").Return(nil).Once()

	require.ErrorContains(t, RestoreSnapshot(ctx, q), "greater than last ingested")

	q.AssertExpectations(t)
}

func TestLoadTestRestoreEqualLedger(t *testing.T) {
	ctx := context.Background()
	q := &mockDBQ{}

	last := uint32(200)
	restore := uint32(200)

	q.On("Begin", ctx).Return(nil).Once()
	q.On("GetLastLedgerIngest", ctx).Return(last, nil).Once()
	q.On("GetLoadTestRestoreState", ctx).Return("rid", restore, nil).Once()

	// When equal, we should NOT delete or update ingest version,
	// but we should clear the state and commit.
	q.On("ClearLoadTestRestoreState", ctx).Return(nil).Once()
	q.On("Commit").Return(nil).Once()
	q.On("Rollback").Return(nil).Once()

	require.NoError(t, RestoreSnapshot(ctx, q))

	q.AssertExpectations(t)
}

func TestCheckPendingLoadTest(t *testing.T) {
	ctx := context.Background()

	// Case 1: no state, no run id -> ok
	q := &mockDBQ{}
	q.On("GetLoadTestRestoreState", ctx).Return("", uint32(0), sql.ErrNoRows).Once()
	l := &loadTestSnapshot{HistoryQ: q}
	require.NoError(t, l.checkPendingLoadTest(ctx))
	q.AssertExpectations(t)

	// Case 2: no state but local run id set -> error
	q = &mockDBQ{}
	q.On("GetLoadTestRestoreState", ctx).Return("", uint32(0), sql.ErrNoRows).Once()
	l = &loadTestSnapshot{HistoryQ: q, runId: "rid"}
	require.ErrorContains(t, l.checkPendingLoadTest(ctx), "expected load test to be active")
	q.AssertExpectations(t)

	// Case 3: state exists with same run id -> ok
	q = &mockDBQ{}
	q.On("GetLoadTestRestoreState", ctx).Return("rid", uint32(123), nil).Once()
	l = &loadTestSnapshot{HistoryQ: q, runId: "rid"}
	require.NoError(t, l.checkPendingLoadTest(ctx))
	q.AssertExpectations(t)

	// Case 4: state exists with different run id -> error
	q = &mockDBQ{}
	q.On("GetLoadTestRestoreState", ctx).Return("other", uint32(123), nil).Once()
	l = &loadTestSnapshot{HistoryQ: q, runId: "rid"}
	require.ErrorContains(t, l.checkPendingLoadTest(ctx), "expected: rid")
	q.AssertExpectations(t)
}
