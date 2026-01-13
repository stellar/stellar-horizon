package ingest

import (
	"context"

	"github.com/stellar/go-stellar-sdk/support/errors"
	"github.com/stellar/stellar-horizon/internal/db2/history"
)

// dbState represents the state of the Horizon database for ingestion.
type dbState int

const (
	dbStateEmpty        dbState = iota // lastIngestedLedger == 0
	dbStateValid                       // Ready for resume
	dbStateNeedsRebuild                // Version mismatch
	dbStateInconsistent                // History/ingest ledger mismatch
)

func (s dbState) String() string {
	switch s {
	case dbStateEmpty:
		return "empty"
	case dbStateValid:
		return "valid"
	case dbStateNeedsRebuild:
		return "needs_rebuild"
	case dbStateInconsistent:
		return "inconsistent"
	default:
		return "unknown"
	}
}

// dbStateInput contains the values needed to evaluate DB state.
// Using a struct prevents accidentally swapping integer arguments.
type dbStateInput struct {
	LastIngestedLedger uint32
	IngestVersion      int
	LastHistoryLedger  uint32
	CurrentVersion     int
}

// evaluateDBState is the SINGLE SOURCE OF TRUTH for determining DB state.
// Both checkDBState() (for LoadTest) and startState.run() (in FSM) call this
// function to ensure the logic is not duplicated and cannot diverge.
//
// Returns:
//   - dbStateEmpty: LastIngestedLedger == 0
//   - dbStateNeedsRebuild: IngestVersion != CurrentVersion
//   - dbStateInconsistent: LastHistoryLedger != LastIngestedLedger (and both != 0)
//   - dbStateValid: all checks pass, ready for resume
func evaluateDBState(input dbStateInput) dbState {
	if input.LastIngestedLedger == 0 {
		return dbStateEmpty
	}

	if input.IngestVersion != input.CurrentVersion {
		return dbStateNeedsRebuild
	}

	// LastHistoryLedger == 0 means no history yet but state was ingested - valid
	// LastHistoryLedger != LastIngestedLedger means inconsistent state
	if input.LastHistoryLedger != 0 && input.LastHistoryLedger != input.LastIngestedLedger {
		return dbStateInconsistent
	}

	return dbStateValid
}

// checkDBState fetches DB values and evaluates state (non-blocking read).
// Used by LoadTest to validate DB before running.
func checkDBState(
	ctx context.Context,
	historyQ history.IngestionQ,
	currentVersion int,
) (dbState, uint32, error) {
	lastIngestedLedger, err := historyQ.GetLastLedgerIngestNonBlocking(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "error getting last ingested ledger")
	}

	ingestVersion, err := historyQ.GetIngestVersion(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "error getting ingest version")
	}

	lastHistoryLedger, err := historyQ.GetLatestHistoryLedger(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "error getting last history ledger")
	}

	state := evaluateDBState(dbStateInput{
		LastIngestedLedger: lastIngestedLedger,
		IngestVersion:      ingestVersion,
		LastHistoryLedger:  lastHistoryLedger,
		CurrentVersion:     currentVersion,
	})
	return state, lastIngestedLedger, nil
}
