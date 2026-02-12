# AGENTS.md — internal/ingest/processors

## Purpose
Transform ledger data into database rows. Two processor types: Change (state) and Transaction (time-series).

## Processor Types
| Interface | Input | Output Tables |
|-----------|-------|---------------|
| `ChangeProcessor` | `ingest.Change` (state delta) | `accounts`, `offers`, `trust_lines`, etc. |
| `LedgerTransactionProcessor` | `ingest.LedgerTransaction` | `history_*` tables |

## Naming Convention
| Suffix | Type | Example |
|--------|------|---------|
| `*_processor.go` | Core processor impl | `accounts_processor.go` |
| `*_change_processor.go` | State table processor | `liquidity_pools_change_processor.go` |
| `*_transaction_processor.go` | History table processor | `claimable_balances_transaction_processor.go` |

## Processor Pattern
```go
type FooProcessor struct {
    batch history.FooBatchInsertBuilder
}

func (p *FooProcessor) ProcessTransaction(lcm xdr.LedgerCloseMeta, tx ingest.LedgerTransaction) error {
    // Extract data from tx
    // p.batch.Add(row)
    return nil
}

func (p *FooProcessor) Flush(ctx context.Context, session db.SessionInterface) error {
    return p.batch.Exec(ctx, session)  // MUST call or data lost
}
```

## Key Processors
| Processor | Tables | Notes |
|-----------|--------|-------|
| `effects_processor.go` | `history_effects` | 80+ effect types, largest processor (~1500 lines) |
| `operations_processor.go` | `history_operations` | One row per operation |
| `transactions_processor.go` | `history_transactions` | Fee bumps handled specially |
| `accounts_processor.go` | `accounts` | State table, change processor |
| `asset_stats_processor.go` | `asset_stats` | Aggregated per-asset metrics |

## Batch Insert Pattern
All processors use loaders for FK resolution:
```go
accountLoader.GetFuture(address)  // Queue
assetLoader.GetFuture(asset)      // Queue
// ... after all txs processed ...
accountLoader.Exec(ctx, session)  // Bulk resolve
```

## Adding New Processor
1. Implement `ChangeProcessor` or `LedgerTransactionProcessor`
2. Register in `processor_runner.go` (change or transaction list)
3. Ensure `Flush()` calls batch builder's `Exec()`

## Anti-patterns
- Forgetting to call `Flush()` → silent data loss
- Processing failed transactions for effects → effects only exist for successful txs
- Modifying processor order without understanding dependencies
