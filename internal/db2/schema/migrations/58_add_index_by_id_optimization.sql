-- +migrate Up

CREATE INDEX "index_history_operation_claimable_balances_on_id" ON history_operation_claimable_balances USING btree (history_claimable_balance_id);
CREATE INDEX "index_history_transaction_claimable_balances_on_id" ON history_transaction_claimable_balances USING btree (history_claimable_balance_id);
CREATE INDEX "index_history_operation_liquidity_pools_on_id" ON history_operation_liquidity_pools USING btree (history_liquidity_pool_id);
CREATE INDEX "index_history_transaction_liquidity_pools_on_id" ON history_transaction_liquidity_pools USING btree (history_liquidity_pool_id);

-- +migrate Down

DROP INDEX "index_history_operation_claimable_balances_on_id";
DROP INDEX "index_history_transaction_claimable_balances_on_id";
DROP INDEX "index_history_operation_liquidity_pools_on_id";
DROP INDEX "index_history_transaction_liquidity_pools_on_id";

