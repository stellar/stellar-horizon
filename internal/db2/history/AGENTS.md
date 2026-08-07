# AGENTS.md — internal/db2/history

## Purpose
Data access layer for all Horizon DB operations. `Q` struct wraps all queries via squirrel builder.

## File Organization
| Pattern | Purpose |
|---------|---------|
| `{entity}.go` | Query methods: `Accounts()`, `Select()`, `InsertX()` |
| `{entity}_batch_insert_builder.go` | Bulk insert interfaces + impls |
| `{entity}_loader.go` | Foreign key lookup caching (AccountLoader, AssetLoader) |
| `mock_*.go` | Test doubles for interfaces |
| `main.go` | Core types: `Q`, EffectType constants, all DB struct definitions |

## Key Types (main.go)
- `Q`: Central query struct. Wraps `db.Session` from go-stellar-sdk
- `EffectType`: 80+ constants for effect categorization (EffectAccountCreated=0, EffectTrade=33, etc.)
- `*BatchInsertBuilder`: Interfaces for bulk upserts during ingestion

## Query Patterns
```go
// Always use Q methods, not raw SQL
q.Accounts().ForAccounts(ctx, addresses)     // Returns AccountsQuery builder
q.Transactions().ForAccount(ctx, address)    // Chainable
q.Select(ctx, &dest, query)                  // Execute and scan
```

## Table Categories
| Category | Tables | Query File |
|----------|--------|------------|
| History (time-series) | `history_transactions`, `history_operations`, `history_effects` | `transaction.go`, `operation.go`, `effect.go` |
| State (cumulative) | `accounts`, `offers`, `trust_lines`, `claimable_balances` | `accounts.go`, `offers.go`, etc. |
| Lookup | `history_accounts`, `history_assets` | `account.go`, `asset.go` |

## Loaders (FK caching)
Used during ingestion to batch-resolve FKs:
```go
loader := history.NewAccountLoader()
future := loader.GetFuture(address)  // Queue lookup
loader.Exec(ctx, session)            // Bulk resolve
id := future.Value()                 // Get cached ID
```

## Critical Constraints
- DO NOT change `key_value.go` constants (`exp_ingest_last_ledger`, `exp_ingest_version`) — migration-sensitive, distributed locking relies on exact key names
- `GetLastLedgerIngest()` uses `SELECT ... FOR UPDATE` — intentional for distributed lock
- Batch insert builders: call `Add()` then `Exec()` — not calling Exec = data loss

## Anti-patterns
- Never bypass `Q` methods with raw queries
- Never modify `bindata.go` — auto-generated from migrations
- Avoid `Q.Clone()` unless you need independent transaction scope
