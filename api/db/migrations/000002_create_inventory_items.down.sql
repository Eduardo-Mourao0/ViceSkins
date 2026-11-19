DROP TABLE IF EXISTS inventory_items;

ALTER TABLE users
    DROP COLUMN IF EXISTS inventory_synced_at,
    DROP COLUMN IF EXISTS inventory_refresh_at;
