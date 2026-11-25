-- Datas sempre do relógio da API (@now): o expires_at é comparado com ele.

-- name: CreateDeposit :one
INSERT INTO deposits (user_id, amount_cents, provider, expires_at, created_at, updated_at)
VALUES (@user_id, @amount_cents, @provider, @expires_at, @now, @now)
RETURNING *;

-- name: SetDepositCharge :one
UPDATE deposits
SET provider_charge_id = @charge_id, br_code = @br_code, updated_at = @now
WHERE id = @id
RETURNING *;

-- Cobrança não pôde ser criada no provedor: o depósito nunca existiu.
-- name: DeleteDeposit :exec
DELETE FROM deposits WHERE id = $1 AND status = 'pending' AND provider_charge_id IS NULL;

-- name: GetDeposit :one
SELECT * FROM deposits WHERE id = $1;

-- name: GetDepositForUpdate :one
SELECT * FROM deposits WHERE id = $1 FOR UPDATE;

-- Pix pago depois de expirar aqui (relógios diferentes, job atrasado) ainda
-- é dinheiro que entrou: credita do mesmo jeito.
-- name: MarkDepositPaid :execrows
UPDATE deposits
SET status = 'paid', paid_at = @now, wallet_entry_id = @wallet_entry_id, updated_at = @now
WHERE id = @id AND status IN ('pending', 'expired');

-- name: MarkDepositExpired :execrows
UPDATE deposits
SET status = 'expired', updated_at = @now
WHERE id = @id AND status = 'pending';

-- name: CountOpenDeposits :one
SELECT COUNT(*) FROM deposits
WHERE user_id = @user_id AND status = 'pending' AND expires_at > @now;

-- O Pix em aberto mais recente: a tela de depósito retoma de onde parou.
-- name: GetOpenDeposit :one
SELECT * FROM deposits
WHERE user_id = @user_id AND status = 'pending' AND expires_at > @now
  AND provider_charge_id IS NOT NULL
ORDER BY id DESC
LIMIT 1;

-- name: ListDepositsToExpire :many
SELECT id FROM deposits
WHERE status = 'pending' AND expires_at <= @now
ORDER BY expires_at
LIMIT @max_rows;
