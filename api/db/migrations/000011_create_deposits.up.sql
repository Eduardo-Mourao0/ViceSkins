-- Depósitos via Pix. Cada um é uma cobrança no provedor de pagamento
-- (simulado por enquanto); o saldo só é creditado quando ela é paga.
CREATE TABLE deposits (
    id                 BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id            BIGINT      NOT NULL REFERENCES users (id),
    amount_cents       BIGINT      NOT NULL CHECK (amount_cents > 0),
    status             TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'expired')),
    provider           TEXT        NOT NULL,
    provider_charge_id TEXT,                 -- preenchido logo após criar a cobrança
    br_code            TEXT        NOT NULL DEFAULT '', -- Pix copia e cola
    expires_at         TIMESTAMPTZ NOT NULL,
    paid_at            TIMESTAMPTZ,
    wallet_entry_id    BIGINT REFERENCES wallet_entries (id),
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,
    -- Pago <=> creditado: nunca um sem o outro.
    CHECK ((status = 'paid') = (paid_at IS NOT NULL AND wallet_entry_id IS NOT NULL))
);

-- Uma cobrança do provedor credita no máximo um depósito.
CREATE UNIQUE INDEX deposits_provider_charge_idx ON deposits (provider, provider_charge_id);
CREATE INDEX deposits_user_idx ON deposits (user_id, id DESC);
CREATE INDEX deposits_pending_expiry_idx ON deposits (expires_at) WHERE status = 'pending';
