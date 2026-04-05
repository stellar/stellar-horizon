# AGENTS.md — internal/ingest

## Purpose
FSM-based ledger ingestion from Stellar Core into Horizon's PostgreSQL.

## Architecture
```
system (main.go) → FSM (fsm.go) → states → processor_runner.go → processors/
                                                              → db2/history/
```

## FSM States (fsm.go)
| State | Purpose | Next |
|-------|---------|------|
| `start` | Entry point. Checks DB version, determines path | build, resume, historyRange, waitForCheckpoint |
| `build` | Initial state from checkpoint archive | resume (success), start (failure) |
| `resume` | Steady-state ledger-by-ledger ingestion | resume (loop), start (error) |
| `historyRange` | Backfill missing time-series data | start |
| `waitForCheckpoint` | Sleep until next 64-ledger checkpoint | start |

## Key Definitions
- **Checkpoint ledger**: `(ledger# + 1) mod 64 == 0` — when history archives publish
- **lastIngestedLedger**: Both cumulative AND time-series data complete
- **lastHistoryLedger**: Time-series only (from `history_ledgers`)
- FSM ensures these stay in sync

## Critical Files
| File | Purpose |
|------|---------|
| `main.go` | `system` struct, `Run()` loop |
| `fsm.go` | State definitions, `stateMachineNode` interface |
| `processor_runner.go` | Runs all processors for a ledger range |
| `verify_range_state.go` | Data integrity verification |

## Running Processors
```go
// processor_runner.go orchestrates
runner.RunAllProcessorsOnLedger(ledger)  // Change + Transaction processors
runner.RunTransactionProcessorsOnLedger(ledger)  // Time-series only
```

## Concurrency Warning
**Only ONE Horizon instance should write to DB globally.** FSM uses `SELECT FOR UPDATE` on `key_value` table for distributed locking.

## Reingestion
```bash
# Backfill range (offline operation)
./stellar-horizon db reingest range 1000 2000

# DO NOT run on production while serving traffic
# Endpoints become unavailable until rebuild complete
```

## Anti-patterns
- Never run multiple ingestion instances against same DB
- Never skip `verify-range` after reingest
- State machine errors → fix root cause, don't restart blindly
