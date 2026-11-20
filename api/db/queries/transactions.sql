-- ===== Compra (tudo dentro de uma transação SQL) =====

-- FOR UPDATE trava a linha até o COMMIT: um segundo comprador da mesma
-- listing fica esperando aqui e, quando passa, já vê status = 'sold'.
-- name: GetListingForUpdate :one
SELECT * FROM listings
WHERE id = $1
FOR UPDATE;

-- name: GetUserBalanceForUpdate :one
SELECT balance_cents FROM users
WHERE id = $1
FOR UPDATE;

-- name: MarkListingSold :execrows
UPDATE listings
SET status = 'sold', updated_at = now()
WHERE id = $1 AND status = 'active';

-- name: MarkListingCancelled :exec
UPDATE listings
SET status = 'cancelled', updated_at = now()
WHERE id = $1;

-- name: CreateTransaction :one
INSERT INTO transactions (listing_id, buyer_id, seller_id, item_id, price_cents, fee_cents)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- ===== Máquina de estados =====

-- name: GetTransaction :one
SELECT * FROM transactions
WHERE id = $1;

-- name: GetTransactionForUpdate :one
SELECT * FROM transactions
WHERE id = $1
FOR UPDATE;

-- O "AND status = @from_status" é a trava final: se outro processo já
-- mudou o status, nenhuma linha é atualizada.
-- name: UpdateTransactionStatus :one
UPDATE transactions
SET status         = @to_status,
    trade_offer_id = COALESCE(sqlc.narg(trade_offer_id), trade_offer_id),
    hold_until     = COALESCE(sqlc.narg(hold_until), hold_until),
    failure_reason = COALESCE(sqlc.narg(failure_reason), failure_reason),
    updated_at     = now()
WHERE id = @id AND status = @from_status
RETURNING *;

-- Durante a proteção de troca o item fica travado (não pode ser anunciado),
-- como na Steam.
-- name: TransferItemOwner :exec
UPDATE inventory_items
SET owner_id = @new_owner_id, tradable = @tradable, updated_at = now()
WHERE id = @item_id;

-- name: SetItemTradable :exec
UPDATE inventory_items
SET tradable = @tradable, updated_at = now()
WHERE id = @item_id;

-- name: CancelActiveListingsForItem :execrows
UPDATE listings
SET status = 'cancelled', updated_at = now()
WHERE item_id = $1 AND status = 'active';

-- ===== Filas dos jobs =====

-- name: ListTransactionIDsByStatus :many
SELECT id FROM transactions
WHERE status = @status
ORDER BY id
LIMIT @max_rows;

-- name: ListDueHolds :many
SELECT id FROM transactions
WHERE status = 'on_hold' AND hold_until <= @now::timestamptz
ORDER BY hold_until
LIMIT @max_rows;
