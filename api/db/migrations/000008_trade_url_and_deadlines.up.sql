-- Trade URL: para onde as ofertas de troca do usuário vão. Validada no Go.
ALTER TABLE users ADD COLUMN trade_url TEXT;

-- buyer_trade_url: cópia da trade URL do comprador no momento da compra.
-- Se ele trocar a URL depois, a entrega em andamento não muda de destino.
-- deadline_at: prazo da etapa atual (vendedor enviar / comprador aceitar).
-- Só vale em pending e sent; nos outros status é ignorado.
ALTER TABLE transactions
    ADD COLUMN buyer_trade_url TEXT,
    ADD COLUMN deadline_at     TIMESTAMPTZ;

-- Transações em andamento de antes desta migration ganham um prazo.
UPDATE transactions
SET deadline_at = updated_at + interval '12 hours'
WHERE status IN ('pending', 'sent');

CREATE INDEX transactions_deadline_idx ON transactions (deadline_at) WHERE status IN ('pending', 'sent');
