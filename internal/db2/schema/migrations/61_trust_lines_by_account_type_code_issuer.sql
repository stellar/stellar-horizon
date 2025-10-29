-- +migrate Up

CREATE INDEX "trust_lines_by_type_code_issuer_account" ON trust_lines USING btree (asset_type, asset_code, asset_issuer, account_id);
DROP INDEX "trust_lines_by_type_code_issuer";

-- +migrate Down

DROP INDEX "trust_lines_by_type_code_issuer_account";
CREATE INDEX "trust_lines_by_type_code_issuer" ON trust_lines USING btree (asset_type, asset_code, asset_issuer);
