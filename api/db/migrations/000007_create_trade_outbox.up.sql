-- Outbox do envio de ofertas de troca.
--
-- A linha é gravada na MESMA transação da compra: se a API cair depois do
-- COMMIT, o pedido de envio não se perde. Um job lê a fila e chama o
-- TradeProvider com retry. A linha é fechada (done_at) quando a transação
-- sai de 'pending', na mesma transação SQL da mudança de status.
CREATE TABLE trade_outbox (
    transaction_id  BIGINT      PRIMARY KEY REFERENCES transactions (id),
    attempts        INT         NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Lease: quem pegou a linha tem até locked_until para terminar. Se o
    -- processo morrer no meio, a linha volta para a fila sozinha.
    locked_until    TIMESTAMPTZ,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    done_at         TIMESTAMPTZ
);

CREATE INDEX trade_outbox_due_idx ON trade_outbox (next_attempt_at) WHERE done_at IS NULL;
