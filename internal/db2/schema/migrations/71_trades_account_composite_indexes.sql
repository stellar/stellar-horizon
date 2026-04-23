-- +migrate Up notransaction

CREATE INDEX CONCURRENTLY IF NOT EXISTS htrd_by_base_account_op_order
  ON history_trades (base_account_id, history_operation_id, "order");

CREATE INDEX CONCURRENTLY IF NOT EXISTS htrd_by_counter_account_op_order
  ON history_trades (counter_account_id, history_operation_id, "order");

-- +migrate Down notransaction

DROP INDEX IF EXISTS htrd_by_counter_account_op_order;
DROP INDEX IF EXISTS htrd_by_base_account_op_order;
