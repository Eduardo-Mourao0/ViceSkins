-- ===== Papéis e conta da plataforma =====

-- role: 'admin' resolve disputas. Sincronizado no boot a partir de ADMIN_STEAM_IDS.
-- is_platform: a conta da própria plataforma no ledger (taxas, garantias).
-- debt_cents: o que o usuário deve à plataforma (estorno que não pôde ser
-- cobrado de volta). Depósitos pagam a dívida primeiro.
ALTER TABLE users
    ADD COLUMN role        TEXT    NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    ADD COLUMN is_platform BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN debt_cents  BIGINT  NOT NULL DEFAULT 0 CHECK (debt_cents >= 0);

-- Só a plataforma pode ficar com saldo negativo: quando garante o pagamento
-- de um vendedor antes de recuperar o dinheiro do comprador.
ALTER TABLE users DROP CONSTRAINT users_balance_cents_check;
ALTER TABLE users ADD CONSTRAINT users_balance_cents_check CHECK (balance_cents >= 0 OR is_platform);
-- O saldo de cada linha do ledger é o de users (a checagem de lá basta).
ALTER TABLE wallet_entries DROP CONSTRAINT wallet_entries_balance_after_cents_check;

CREATE UNIQUE INDEX users_one_platform ON users (is_platform) WHERE is_platform;

INSERT INTO users (steam_id, display_name, is_platform)
VALUES ('platform', 'ViceSkins', true);

-- ===== Ledger: dupla entrada para o dinheiro da plataforma =====
--   fee              +  plataforma recebe a taxa de uma venda concluída
--   dispute_charge   -  comprador: cobrança de um item recebido depois do estorno
--   dispute_collect  +  plataforma recebe essa cobrança
--   dispute_payout   -  plataforma paga o vendedor da disputa (garantia)
--   dispute_refund   ±  disputa desfeita: cobrança devolvida ao comprador
--   debt_payment     -  comprador paga a dívida (no depósito)
--   debt_recovery    +  plataforma recebe o pagamento da dívida
ALTER TABLE wallet_entries DROP CONSTRAINT wallet_entries_kind_check;
ALTER TABLE wallet_entries ADD CONSTRAINT wallet_entries_kind_check CHECK (kind IN (
    'deposit', 'purchase', 'sale', 'refund', 'fee',
    'dispute_charge', 'dispute_collect', 'dispute_payout', 'dispute_refund',
    'debt_payment', 'debt_recovery'));

-- ===== Disputas =====

-- Aberta quando o item aparece com o comprador depois do estorno. A cobrança
-- é automática; o vendedor recebe depois da proteção de troca, como numa
-- venda normal. Um admin pode arquivar (desfazer) se a detecção estiver errada.
CREATE TABLE disputes (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    transaction_id BIGINT      NOT NULL UNIQUE REFERENCES transactions (id),
    status         TEXT        NOT NULL DEFAULT 'holding'
                   CHECK (status IN ('holding', 'settled', 'reversed', 'dismissed')),
    -- Evidência
    detected_at    TIMESTAMPTZ NOT NULL,
    refunded_at    TIMESTAMPTZ NOT NULL, -- quando a compra foi estornada
    evidence       TEXT        NOT NULL CHECK (evidence IN ('fingerprint', 'offer')),
    traded_at      TIMESTAMPTZ,          -- horário real da troca, se a Steam informou
    -- Dinheiro
    charged_cents  BIGINT      NOT NULL CHECK (charged_cents >= 0), -- cobrado do saldo do comprador
    covered_cents  BIGINT      NOT NULL CHECK (covered_cents >= 0), -- virou dívida (a plataforma garante)
    hold_until     TIMESTAMPTZ NOT NULL,
    -- Resolução
    resolved_at    TIMESTAMPTZ,
    resolved_by    BIGINT REFERENCES users (id), -- admin; NULL = automática
    note           TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX disputes_holding_idx ON disputes (hold_until) WHERE status = 'holding';
