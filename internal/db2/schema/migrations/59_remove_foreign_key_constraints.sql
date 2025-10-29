-- +migrate Up

ALTER TABLE ONLY history_trades DROP CONSTRAINT history_trades_base_account_id_fkey;
ALTER TABLE ONLY history_trades DROP CONSTRAINT history_trades_base_asset_id_fkey;
ALTER TABLE ONLY history_trades DROP CONSTRAINT history_trades_counter_account_id_fkey;
ALTER TABLE ONLY history_trades DROP CONSTRAINT history_trades_counter_asset_id_fkey;

-- +migrate Down

ALTER TABLE ONLY history_trades
    ADD CONSTRAINT history_trades_base_account_id_fkey FOREIGN KEY (base_account_id) REFERENCES history_accounts(id);

ALTER TABLE ONLY history_trades
    ADD CONSTRAINT history_trades_base_asset_id_fkey FOREIGN KEY (base_asset_id) REFERENCES history_assets(id);

ALTER TABLE ONLY history_trades
    ADD CONSTRAINT history_trades_counter_account_id_fkey FOREIGN KEY (counter_account_id) REFERENCES history_accounts(id);

ALTER TABLE ONLY history_trades
    ADD CONSTRAINT history_trades_counter_asset_id_fkey FOREIGN KEY (counter_asset_id) REFERENCES history_assets(id); 
