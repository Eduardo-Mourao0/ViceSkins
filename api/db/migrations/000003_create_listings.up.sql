CREATE TABLE listings (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    item_id     BIGINT      NOT NULL REFERENCES inventory_items (id),
    seller_id   BIGINT      NOT NULL REFERENCES users (id),
    price_cents BIGINT      NOT NULL CHECK (price_cents > 0),
    status      TEXT        NOT NULL DEFAULT 'active'
                CHECK (status IN ('active', 'sold', 'cancelled')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Um item só pode ter UMA listing ativa. O banco garante isso mesmo
-- com duas requisições simultâneas; listings antigas (sold/cancelled) não contam.
CREATE UNIQUE INDEX listings_one_active_per_item ON listings (item_id) WHERE status = 'active';

CREATE INDEX listings_seller_idx ON listings (seller_id, created_at DESC);
CREATE INDEX listings_active_created_idx ON listings (created_at DESC) WHERE status = 'active';
