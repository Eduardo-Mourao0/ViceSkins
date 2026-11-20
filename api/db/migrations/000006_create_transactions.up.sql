CREATE TABLE transactions (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    listing_id     BIGINT      NOT NULL UNIQUE REFERENCES listings (id), -- uma listing vende uma vez só
    buyer_id       BIGINT      NOT NULL REFERENCES users (id),
    seller_id      BIGINT      NOT NULL REFERENCES users (id),
    item_id        BIGINT      NOT NULL REFERENCES inventory_items (id),
    price_cents    BIGINT      NOT NULL CHECK (price_cents > 0),
    fee_cents      BIGINT      NOT NULL CHECK (fee_cents >= 0 AND fee_cents <= price_cents),
    status         TEXT        NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending', 'sent', 'on_hold', 'completed', 'cancelled', 'failed')),
    trade_offer_id TEXT,
    hold_until     TIMESTAMPTZ,
    failure_reason TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (buyer_id <> seller_id)
);

CREATE INDEX transactions_buyer_idx  ON transactions (buyer_id, created_at DESC);
CREATE INDEX transactions_seller_idx ON transactions (seller_id, created_at DESC);
-- Filas dos jobs em background.
CREATE INDEX transactions_sent_idx    ON transactions (id) WHERE status = 'sent';
CREATE INDEX transactions_on_hold_idx ON transactions (hold_until) WHERE status = 'on_hold';

ALTER TABLE wallet_entries
    ADD CONSTRAINT wallet_entries_transaction_fk FOREIGN KEY (transaction_id) REFERENCES transactions (id);
