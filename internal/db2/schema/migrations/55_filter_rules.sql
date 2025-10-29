-- +migrate Up

CREATE TABLE account_filter_rules (
    enabled bool NOT NULL default false,
    whitelist varchar[] NOT NULL,
    last_modified bigint NOT NULL
);

CREATE TABLE asset_filter_rules (
    enabled bool NOT NULL default false,
    whitelist varchar[] NOT NULL,
    last_modified bigint NOT NULL
);

-- insert the default disabled state for each supported filter implementation
INSERT INTO account_filter_rules VALUES (false, '{}', 0);
INSERT INTO asset_filter_rules VALUES (false, '{}', 0);

CREATE TABLE txsub_results (
    transaction_hash       varchar(64) NOT NULL UNIQUE,
    inner_transaction_hash varchar(64),
    tx_result              text, -- serialized history.Transaction
    submitted_at           timestamp NOT NULL DEFAULT NOW()
);

-- +migrate Down

DROP TABLE account_filter_rules cascade;
DROP TABLE asset_filter_rules cascade;
DROP TABLE txsub_results cascade;
