# AGENTS.md — internal/actions

## Purpose
HTTP handlers for Horizon REST API. One file per resource type.

## Handler Pattern
```go
// account.go
func GetAccountByID(w http.ResponseWriter, r *http.Request) {
    // 1. Parse params
    accountID, err := getAccountID(r, "account_id")
    
    // 2. Query DB
    historyQ, _ := horizonContext.HistoryQFromRequest(r)
    account, err := historyQ.Accounts().ForAccounts(ctx, []string{accountID}).Select(ctx)
    
    // 3. Adapt to resource
    resource := resourceadapter.NewAccount(account)
    
    // 4. Render
    httpjson.Render(w, resource, httpjson.HALJSON)
}
```

## File Organization
| File | Endpoints |
|------|-----------|
| `account.go` | `/accounts/{id}`, `/accounts` |
| `transaction.go` | `/transactions/{hash}`, `/transactions` |
| `operation.go` | `/operations/{id}`, `/operations` |
| `ledger.go` | `/ledgers/{seq}`, `/ledgers` |
| `offer.go` | `/offers/{id}`, `/accounts/{id}/offers` |
| `trade.go` | `/trades`, `/accounts/{id}/trades` |
| `path.go` | `/paths/strict-receive`, `/paths/strict-send` |
| `submit_transaction.go` | `POST /transactions` |
| `submit_transaction_async.go` | `POST /transactions_async` |

## Helper Files
| File | Purpose |
|------|---------|
| `helpers.go` | Param parsing: `getString()`, `getAccountID()`, `getCursor()` |
| `validators.go` | Input validation |
| `query_params.go` | URL query struct definitions |
| `main.go` | Shared types, `ActionContext` |

## Response Formats
- **JSON**: Standard responses via `httpjson.Render()`
- **SSE**: Streaming via `sse.Stream()` for `/stream` suffix endpoints

## Pagination Pattern
```go
// GetPageQuery returns cursor, order, limit
pq, _ := GetPageQuery(r, opts...)
query := q.Transactions().Page(pq)
```

## Context Access
```go
historyQ, _ := horizonContext.HistoryQFromRequest(r)    // DB queries
ledgerState := r.Context().Value(&ledger.State{})       // Current ledger
app := r.Context().Value(&App{})                        // Full app access
```

## Error Handling
Use `support/render/problem` for RFC 7807 responses:
```go
problem.Render(ctx, w, problem.NotFound)
problem.Render(ctx, w, hProblem.StaleHistory)  // Horizon-specific
```

## Anti-patterns
- Raw SQL in handlers → use `historyQ` methods
- Blocking in SSE handlers → respect context cancellation
- Ignoring `Latest-Ledger` header → always set via `SetLastLedgerHeader()`
