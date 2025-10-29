-- +migrate Up

CREATE INDEX IF NOT EXISTS htrd_agg_timestamp ON history_trades_60000 USING btree (timestamp);

DROP INDEX IF EXISTS htrd_agg_timestamp_brin;

-- +migrate Down

CREATE INDEX IF NOT EXISTS htrd_agg_timestamp_brin ON history_trades_60000 USING brin(timestamp);

DROP INDEX IF EXISTS htrd_agg_timestamp;
