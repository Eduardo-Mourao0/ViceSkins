DROP INDEX IF EXISTS transactions_closed_watch_idx;
DROP INDEX IF EXISTS transactions_watch_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS late_delivery_at, DROP COLUMN IF EXISTS last_checked_at;
ALTER TABLE users DROP COLUMN IF EXISTS steam_api_key_last4, DROP COLUMN IF EXISTS steam_api_key_enc;
