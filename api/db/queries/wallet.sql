-- O UPDATE trava a linha do usuário até o fim da transação: dois depósitos
-- simultâneos ficam em fila, e cada um vê o saldo já atualizado pelo outro.
-- name: AddToBalance :one
UPDATE users
SET balance_cents = balance_cents + @amount_cents,
    updated_at    = now()
WHERE id = @user_id
RETURNING balance_cents;

-- name: InsertWalletEntry :one
INSERT INTO wallet_entries (user_id, amount_cents, balance_after_cents, kind, transaction_id, description)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: ListWalletEntries :many
SELECT *
FROM wallet_entries
WHERE user_id = $1
ORDER BY id DESC
LIMIT $2;

-- Auditoria: saldo guardado x soma do ledger. Devem ser sempre iguais.
-- name: GetWalletAudit :one
SELECT users.balance_cents,
       COALESCE((SELECT SUM(amount_cents) FROM wallet_entries WHERE user_id = users.id), 0)::bigint AS ledger_sum_cents,
       (SELECT COUNT(*) FROM wallet_entries WHERE user_id = users.id) AS entry_count
FROM users
WHERE users.id = $1;
