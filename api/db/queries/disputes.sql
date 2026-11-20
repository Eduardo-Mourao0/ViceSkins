-- ===== Conta da plataforma, dívida e papéis =====

-- name: GetPlatformAccountID :one
SELECT id FROM users WHERE is_platform;

-- Trava a linha do usuário (saldo e dívida) até o fim da transação.
-- name: GetUserMoneyForUpdate :one
SELECT balance_cents, debt_cents FROM users
WHERE id = $1
FOR UPDATE;

-- amount negativo reduz a dívida (o CHECK impede passar de zero).
-- name: AddDebt :exec
UPDATE users SET debt_cents = debt_cents + @amount_cents, updated_at = now()
WHERE id = @user_id;

-- Admins vêm de ADMIN_STEAM_IDS, aplicado no boot: tirar alguém da lista
-- tira o papel dele.
-- name: SyncAdmins :exec
UPDATE users
SET role = CASE WHEN steam_id = ANY (@steam_ids::text[]) THEN 'admin' ELSE 'user' END
WHERE NOT is_platform;

-- ===== Disputas =====

-- name: CreateDispute :one
INSERT INTO disputes (transaction_id, detected_at, refunded_at, evidence, traded_at,
                      charged_cents, covered_cents, hold_until)
VALUES (@transaction_id, @detected_at::timestamptz, @refunded_at::timestamptz, @evidence, sqlc.narg(traded_at),
        @charged_cents, @covered_cents, @hold_until::timestamptz)
RETURNING *;

-- name: GetDisputeForUpdate :one
SELECT * FROM disputes
WHERE id = $1
FOR UPDATE;

-- name: ListDueDisputes :many
SELECT id FROM disputes
WHERE status = 'holding' AND hold_until <= @now::timestamptz
ORDER BY hold_until
LIMIT @max_rows;

-- O "AND status = 'holding'" é a trava final, como na máquina de estados.
-- name: ResolveDispute :execrows
UPDATE disputes
SET status = @status, resolved_at = @now::timestamptz, resolved_by = sqlc.narg(resolved_by), note = sqlc.narg(note)
WHERE id = @id AND status = 'holding';

-- Lista para o admin: disputa + transação + item + as duas partes.
-- name: ListDisputes :many
SELECT sqlc.embed(disputes),
       sqlc.embed(transactions),
       inventory_items.name       AS item_name,
       inventory_items.icon_url   AS item_icon_url,
       buyer.display_name         AS buyer_name,
       buyer.debt_cents           AS buyer_debt_cents,
       seller.display_name        AS seller_name
FROM disputes
JOIN transactions    ON transactions.id = disputes.transaction_id
JOIN inventory_items ON inventory_items.id = transactions.item_id
JOIN users AS buyer  ON buyer.id = transactions.buyer_id
JOIN users AS seller ON seller.id = transactions.seller_id
WHERE (sqlc.narg(status)::text IS NULL OR disputes.status = sqlc.narg(status)::text)
ORDER BY disputes.id DESC
LIMIT @max_rows;
