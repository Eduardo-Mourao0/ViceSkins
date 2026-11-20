-- API key da Steam do vendedor (opcional), criptografada com AES-GCM.
-- A chave mestra fica fora do banco (SECRETS_KEY). last4 só para exibir.
ALTER TABLE users
    ADD COLUMN steam_api_key_enc   BYTEA,
    ADD COLUMN steam_api_key_last4 TEXT;

-- last_checked_at: última verificação da troca na Steam (o job revisa cada
-- transação no máximo a cada TRADE_CHECK_INTERVAL, por causa do rate limit).
-- late_delivery_at: o item apareceu com o comprador DEPOIS do estorno
-- (aceitou uma oferta que ficou aberta). Fica marcada para análise manual.
ALTER TABLE transactions
    ADD COLUMN last_checked_at  TIMESTAMPTZ,
    ADD COLUMN late_delivery_at TIMESTAMPTZ;

CREATE INDEX transactions_watch_idx ON transactions (last_checked_at NULLS FIRST)
    WHERE status IN ('pending', 'sent', 'on_hold');
CREATE INDEX transactions_closed_watch_idx ON transactions (updated_at)
    WHERE status IN ('cancelled', 'failed') AND late_delivery_at IS NULL;
