# AGENTS.md — stellar-horizon

## Overview
Horizon is the client-facing HTTP API for the Stellar blockchain network. Ingests ledger data from Stellar Core, serves REST/SSE endpoints, persists to PostgreSQL.

## Architecture
```
main.go → cmd/root.go (cobra CLI) → internal/flags.go (NewAppFromFlags)
    → internal/app.go (App) → internal/httpx/server.go (HTTP server)
                            → internal/ingest/ (FSM-based ledger processor)
                            → internal/txsub/ (transaction submission)
```

### Core Subsystems
| Package | Purpose | Entry |
|---------|---------|-------|
| `internal/ingest/` | FSM ledger ingestion. States: start→build→resume loop | `main.go`, `fsm.go` |
| `internal/db2/history/` | DB models + queries. `Q` struct wraps all history ops | `main.go` |
| `internal/actions/` | HTTP handlers. One file per resource type | `account.go`, `transaction.go`, etc. |
| `internal/txsub/` | Tx submission to Core, result tracking | `submitter.go` |
| `internal/httpx/` | Router setup, middleware, server lifecycle | `server.go`, `router.go` |

### Key Types
- `App` (internal/app.go): Main application container. Holds web server, ingester, submitter, DB sessions
- `Q` (internal/db2/history/main.go): Query builder for all history tables. Wraps squirrel
- `system` (internal/ingest/main.go): Ingestion coordinator. Runs FSM, holds processors

## Build & Run
```bash
# Build
go build -o stellar-horizon -trimpath -v .

# Local dev (requires Docker)
./docker/start.sh standalone   # or: testnet, pubnet

# CLI commands
./stellar-horizon serve        # Start API server
./stellar-horizon db migrate up
./stellar-horizon ingest verify-range 1000 2000
```

## Testing
```bash
go test ./...                              # Unit tests (co-located *_test.go)
HORIZON_INTEGRATION_TESTS_ENABLED=true \   # Integration tests
  go test -v ./internal/integration/...

# Integration test setup requires:
# - PostgreSQL (HORIZON_INTEGRATION_TESTS_DOCKER_IMG or local)
# - Stellar Core binary (STELLAR_CORE_BINARY_PATH)
```
DO NOT use `internal/test/integration/` framework — deprecated, will be EOL.

## Code Style
**Linting**: golangci-lint with `.golangci.yml`
- Max line length: 140 chars
- Max function length: 100 lines, 50 statements  
- Cyclomatic complexity: 15
- Run: `golangci-lint run` or `./gofmt.sh && ./govet.sh && ./staticcheck.sh`

**Formatting**: gofmt + goimports (local prefix: `github.com/stellar/stellar-horizon`)

## Database
PostgreSQL. Migrations in `internal/db2/schema/migrations/` (goose-style numbered SQL).

Two table categories:
1. **History tables**: Time-series data (`history_transactions`, `history_operations`, `history_effects`)
2. **State tables**: Current ledger state (`accounts`, `offers`, `trust_lines`, `claimable_balances`)

Generated bindata: `internal/db2/schema/bindata.go` — DO NOT EDIT manually.

## Ingest FSM (internal/ingest/)
State machine for ledger processing:
```
start → build (checkpoint) → resume (ledger-by-ledger) ↺
      → historyRange (backfill gaps)
      → waitForCheckpoint (if ahead of archives)
```
- Checkpoint ledger: `(ledger# + 1) mod 64 == 0`
- `lastIngestedLedger`: cumulative + time-series data complete
- `lastHistoryLedger`: time-series only (from `history_ledgers` table)

Critical: Only ONE instance should write to DB at a time globally.

## API Endpoints (internal/actions/)
Handlers return JSON or SSE streams. Pattern:
```go
func GetAccount(w http.ResponseWriter, r *http.Request) {
    // 1. Parse/validate params via helpers.go
    // 2. Query via historyQ (db2/history)
    // 3. Adapt to resource via resourceadapter/
    // 4. Render JSON or stream SSE
}
```

## Dependencies (go.mod)
| Package | Use |
|---------|-----|
| `github.com/stellar/go-stellar-sdk` | XDR, ingest, history archives |
| `github.com/go-chi/chi` | HTTP router |
| `github.com/Masterminds/squirrel` | SQL query builder |
| `github.com/spf13/cobra` | CLI framework |
| `github.com/prometheus/client_golang` | Metrics |

## Key Files
| File | Purpose |
|------|---------|
| `internal/flags.go` | All CLI flags, env vars. DEPRECATED flags documented inline |
| `internal/app.go` | App lifecycle: init, serve, tick, shutdown |
| `internal/ingest/fsm.go` | FSM state definitions and transitions |
| `internal/db2/history/main.go` | All DB types (1295 lines). Central data model |
| `internal/httpx/router.go` | Route definitions, middleware chain |

## Conventions
- Errors: Use `github.com/stellar/go-stellar-sdk/support/errors` for wrapping
- Logging: `github.com/stellar/go-stellar-sdk/support/log` (structured)
- Problems: RFC 7807 via `internal/render/problem/` and `support/render/problem`
- Context: Pass `context.Context` first param, respect cancellation

## Anti-patterns (from codebase)
- NEVER run `db reingest` on production DB without planning for endpoint unavailability
- DO NOT change key_value store keys in `internal/db2/history/key_value.go` — migration-sensitive
- DO NOT modify `bindata.go` files — auto-generated

## See Also
- `DEVELOPING.md`: Full dev environment setup
- `CONTRIBUTING.md`: PR workflow
- `internal/docs/TESTING_NOTES.md`: Test patterns and gotchas
- `internal/ingest/README.md`: FSM deep dive with state diagram
