DROP INDEX IF EXISTS transactions_deadline_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS deadline_at, DROP COLUMN IF EXISTS buyer_trade_url;
ALTER TABLE users DROP COLUMN IF EXISTS trade_url;
