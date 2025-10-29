-- +migrate Up

CREATE INDEX "htrd_by_counter_asset" ON history_trades USING btree (counter_asset_id);
CREATE INDEX "htrd_agg_counter_asset" ON history_trades_60000 USING btree (counter_asset_id);

-- +migrate Down

DROP INDEX "htrd_by_counter_asset";
DROP INDEX "htrd_agg_counter_asset";
