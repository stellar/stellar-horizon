-- +migrate Up

alter table history_trades_60000 set (
  autovacuum_vacuum_scale_factor = 0.05,
  autovacuum_analyze_scale_factor = 0.025
);

-- +migrate Down

alter table history_trades_60000 set (
  autovacuum_vacuum_scale_factor = 0.1,
  autovacuum_analyze_scale_factor = 0.5
);
