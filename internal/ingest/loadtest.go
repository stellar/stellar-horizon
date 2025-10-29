package ingest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/stellar/go/services/horizon/internal/db2/history"
	"github.com/stellar/go/toid"
)

type loadTestSnapshot struct {
	HistoryQ history.IngestionQ
	runId    string
}

func (l *loadTestSnapshot) checkPendingLoadTest(ctx context.Context) error {
	if runID, _, err := l.HistoryQ.GetLoadTestRestoreState(ctx); errors.Is(err, sql.ErrNoRows) {
		if l.runId != "" {
			return fmt.Errorf("expected load test to be active with run id: %s", l.runId)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("error getting load test restore state: %w", err)
	} else if runID != l.runId {
		return fmt.Errorf("load test run id is %s, expected: %s", runID, l.runId)
	}
	return nil
}

func (l *loadTestSnapshot) save(ctx context.Context) error {
	if err := l.HistoryQ.Begin(ctx); err != nil {
		return fmt.Errorf("error starting a transaction: %w", err)
	}
	defer l.HistoryQ.Rollback()
	if l.runId != "" {
		return fmt.Errorf("load test already active, run id: %s", l.runId)
	}

	// This will get the value `FOR UPDATE`, blocking it for other nodes.
	lastIngestedLedger, err := l.HistoryQ.GetLastLedgerIngest(ctx)
	if err != nil {
		return fmt.Errorf("error getting last ledger ingested: %w", err)
	}

	runID, restoreLedger, err := l.HistoryQ.GetLoadTestRestoreState(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		// No active load test state; create one with a random runID
		buf := make([]byte, 16)
		if _, err = rand.Read(buf); err != nil {
			return fmt.Errorf("error generating runID: %w", err)
		}
		runID = hex.EncodeToString(buf)
		if err = l.HistoryQ.SetLoadTestRestoreState(ctx, runID, lastIngestedLedger); err != nil {
			return fmt.Errorf("error setting load test restore state: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("error getting load test restore state: %w", err)
	} else {
		return fmt.Errorf("load test already active, restore ledger: %d, run id: %s", restoreLedger, runID)
	}

	if err = l.HistoryQ.Commit(); err != nil {
		return fmt.Errorf("error committing a transaction: %w", err)
	}
	l.runId = runID
	return nil
}

// RestoreSnapshot reverts the state of the horizon db to a previous snapshot recorded at the start of an
// ingestion load test.
var RestoreSnapshot = restoreSnapshot

func restoreSnapshot(ctx context.Context, historyQ history.IngestionQ) error {
	if err := historyQ.Begin(ctx); err != nil {
		return fmt.Errorf("error starting a transaction: %w", err)
	}
	defer historyQ.Rollback()

	// This will get the value `FOR UPDATE`, blocking it for other nodes.
	lastIngestedLedger, err := historyQ.GetLastLedgerIngest(ctx)
	if err != nil {
		return fmt.Errorf("error getting last ledger ingested: %w", err)
	}

	_, restoreLedger, err := historyQ.GetLoadTestRestoreState(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return fmt.Errorf("error getting load test restore ledger: %w", err)
	}

	if restoreLedger > lastIngestedLedger {
		return fmt.Errorf("load test restore ledger: %d is greater than last ingested ledger: %d", restoreLedger, lastIngestedLedger)
	}

	if restoreLedger < lastIngestedLedger {
		var start, end int64
		start, end, err = toid.LedgerRangeInclusive(
			int32(restoreLedger+1),
			int32(lastIngestedLedger),
		)
		if err != nil {
			return fmt.Errorf("invalid range: %w", err)
		}

		if _, err = historyQ.DeleteRangeAll(ctx, start, end); err != nil {
			return fmt.Errorf("error deleting range all: %w", err)
		}

		if err = historyQ.UpdateIngestVersion(ctx, 0); err != nil {
			return fmt.Errorf("error updating ingestion version: %w", err)
		}
	}

	if err = historyQ.ClearLoadTestRestoreState(ctx); err != nil {
		return fmt.Errorf("error clearing load test restore ledger: %w", err)
	}

	if err = historyQ.Commit(); err != nil {
		return fmt.Errorf("error committing a transaction: %w", err)
	}
	return nil
}
