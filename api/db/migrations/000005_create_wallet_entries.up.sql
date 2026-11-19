-- Ledger da carteira: cada crédito/débito é uma linha imutável.
-- Invariante: users.balance_cents = SUM(wallet_entries.amount_cents) do usuário.
CREATE TABLE wallet_entries (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id             BIGINT      NOT NULL REFERENCES users (id),
    amount_cents        BIGINT      NOT NULL CHECK (amount_cents <> 0), -- > 0 crédito, < 0 débito
    balance_after_cents BIGINT      NOT NULL CHECK (balance_after_cents >= 0),
    kind                TEXT        NOT NULL CHECK (kind IN ('deposit', 'purchase', 'sale', 'refund')),
    transaction_id      BIGINT,     -- FK para transactions entra na etapa 6
    description         TEXT        NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX wallet_entries_user_idx ON wallet_entries (user_id, id DESC);
