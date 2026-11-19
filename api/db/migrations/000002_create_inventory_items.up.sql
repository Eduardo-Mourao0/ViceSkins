ALTER TABLE users
    ADD COLUMN inventory_refresh_at TIMESTAMPTZ, -- última tentativa (controla o cooldown)
    ADD COLUMN inventory_synced_at  TIMESTAMPTZ; -- último sync bem-sucedido

CREATE TABLE inventory_items (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    owner_id         BIGINT      NOT NULL REFERENCES users (id),
    asset_id         TEXT        NOT NULL UNIQUE, -- muda a cada troca na Steam
    class_id         TEXT        NOT NULL,
    instance_id      TEXT        NOT NULL,
    market_hash_name TEXT        NOT NULL,
    name             TEXT        NOT NULL,
    item_type        TEXT        NOT NULL,
    weapon           TEXT        NOT NULL DEFAULT '',
    rarity           TEXT        NOT NULL DEFAULT '',
    rarity_color     TEXT        NOT NULL DEFAULT '',
    exterior         TEXT        NOT NULL DEFAULT '',
    float_value      DOUBLE PRECISION,           -- NULL para itens sem desgaste (caixas, agentes...)
    paint_seed       INTEGER,
    icon_url         TEXT        NOT NULL,
    stickers         JSONB       NOT NULL DEFAULT '[]',
    tradable         BOOLEAN     NOT NULL,
    marketable       BOOLEAN     NOT NULL,
    removed_at       TIMESTAMPTZ,                -- saiu do inventário na Steam
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX inventory_items_owner_active_idx ON inventory_items (owner_id) WHERE removed_at IS NULL;
