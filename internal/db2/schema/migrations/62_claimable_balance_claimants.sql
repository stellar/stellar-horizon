-- +migrate Up

CREATE TABLE claimable_balance_claimants (
    id TEXT NOT NULL, -- ClaimableBalanceID in base64
    destination character varying(56) NOT NULL,
    last_modified_ledger integer NOT NULL,
    PRIMARY KEY (id, destination)
);

CREATE INDEX "claimable_balance_claimants_by_destination_last_modified_ledger_id" ON claimable_balance_claimants USING btree (destination, last_modified_ledger, id);

/* Supports "select * from claimable_balances where asset = ? order by last_modified_ledger, id" */
CREATE INDEX "claimable_balances_by_asset_last_modified_ledger_id" ON claimable_balances USING btree (asset, last_modified_ledger, id);
/* Remove index replaced by above */
DROP INDEX "claimable_balances_by_asset";

/* Supports "select * from claimable_balances where sponsor = ? order by last_modified_ledger, id" */
CREATE INDEX "claimable_balances_by_sponsor_last_modified_ledger_id" ON claimable_balances USING btree (sponsor, last_modified_ledger, id);
/* Remove index replaced by above */
DROP INDEX "claimable_balances_by_sponsor";

-- +migrate Down

DROP TABLE claimable_balance_claimants cascade;

CREATE INDEX "claimable_balances_by_asset" ON claimable_balances USING btree (asset);
DROP INDEX "claimable_balances_by_asset_last_modified_ledger_id";

CREATE INDEX "claimable_balances_by_sponsor" ON claimable_balances USING btree (sponsor);
DROP INDEX "claimable_balances_by_sponsor_last_modified_ledger_id";

