-- +migrate Up

-- Add is_dust to trades table
ALTER TABLE history_trades ADD rounding_slippage bigint;
ALTER TABLE history_trades ADD base_is_exact boolean;

-- +migrate Down
ALTER TABLE history_trades DROP rounding_slippage;
ALTER TABLE history_trades DROP base_is_exact;
