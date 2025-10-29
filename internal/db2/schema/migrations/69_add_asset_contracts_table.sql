-- +migrate Up

ALTER TABLE exp_asset_stats DROP COLUMN contract_id CASCADE;

CREATE TABLE asset_contracts (
     key_hash        BYTEA NOT NULL,
     contract_id     BYTEA NOT NULL,
     asset_type      INT NOT NULL,
     asset_code      VARCHAR(12) NOT NULL,
     asset_issuer    VARCHAR(56) NOT NULL,
     expiration_ledger integer NOT NULL,
     PRIMARY KEY(asset_code, asset_issuer, asset_type),
     UNIQUE(key_hash),
     UNIQUE(contract_id)
);

CREATE INDEX "asset_contracts_by_expiration" ON asset_contracts USING btree (expiration_ledger);


-- +migrate Down

ALTER TABLE exp_asset_stats ADD COLUMN contract_id BYTEA UNIQUE;

DROP TABLE asset_contracts cascade;