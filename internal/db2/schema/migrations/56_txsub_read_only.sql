-- +migrate Up

DROP TABLE txsub_results;

-- used to temporarily store filtered-out transactions
-- needed by the transaction system
--  
-- any future migrations that would change history_transactions
-- require dropping history_transactions_filtered_tmp, and rerunning this CREATE..SELECT..AS
-- to keep the tmp table in sync with schema.
CREATE TABLE history_transactions_filtered_tmp AS 
  select * FROM history_transactions
  WHERE ledger_sequence IS NULL;

-- +migrate Down

DROP TABLE history_transactions_filtered_tmp;

CREATE TABLE txsub_results (
    transaction_hash       varchar(64) NOT NULL UNIQUE,
    inner_transaction_hash varchar(64),
    tx_result              text, -- serialized history.Transaction
    submitted_at           timestamp NOT NULL DEFAULT NOW()
);
