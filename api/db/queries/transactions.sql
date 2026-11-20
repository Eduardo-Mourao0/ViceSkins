-- ===== Compra (tudo dentro de uma transação SQL) =====

-- FOR UPDATE trava a linha até o COMMIT: um segundo comprador da mesma
-- listing fica esperando aqui e, quando passa, já vê status = 'sold'.
-- name: GetListingForUpdate :one
SELECT * FROM listings
WHERE id = $1
FOR UPDATE;

-- name: GetBuyerForUpdate :one
SELECT balance_cents, trade_url FROM users
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
INSERT INTO transactions (listing_id, buyer_id, seller_id, item_id, price_cents, fee_cents, buyer_trade_url, deadline_at)
VALUES (@listing_id, @buyer_id, @seller_id, @item_id, @price_cents, @fee_cents, @buyer_trade_url, @deadline_at::timestamptz)
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
    deadline_at    = COALESCE(sqlc.narg(deadline_at), deadline_at),
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

-- Transações que perderam o prazo da etapa atual.
-- name: ListExpiredTransactions :many
SELECT id FROM transactions
WHERE status IN ('pending', 'sent') AND deadline_at <= @now::timestamptz
ORDER BY deadline_at
LIMIT @max_rows;

-- ===== Histórico =====

-- Transações em que o usuário é comprador ou vendedor, com item e as duas partes.
-- Paginação por cursor: "before_id" é o menor id da página anterior.
-- name: ListUserTransactions :many
SELECT sqlc.embed(transactions),
       sqlc.embed(inventory_items),
       buyer.display_name  AS buyer_name,
       buyer.avatar_url    AS buyer_avatar_url,
       seller.display_name AS seller_name,
       seller.avatar_url   AS seller_avatar_url
FROM transactions
JOIN inventory_items ON inventory_items.id = transactions.item_id
JOIN users AS buyer  ON buyer.id = transactions.buyer_id
JOIN users AS seller ON seller.id = transactions.seller_id
WHERE CASE sqlc.narg(role)::text
          WHEN 'buyer'  THEN transactions.buyer_id = @user_id
          WHEN 'seller' THEN transactions.seller_id = @user_id
          ELSE transactions.buyer_id = @user_id OR transactions.seller_id = @user_id
      END
  AND (cardinality(@statuses::text[]) = 0 OR transactions.status = ANY (@statuses::text[]))
  AND (sqlc.narg(before_id)::bigint IS NULL OR transactions.id < sqlc.narg(before_id)::bigint)
ORDER BY transactions.id DESC
LIMIT @max_rows;

-- name: GetTransactionDetail :one
SELECT sqlc.embed(transactions),
       sqlc.embed(inventory_items),
       buyer.display_name  AS buyer_name,
       buyer.avatar_url    AS buyer_avatar_url,
       seller.display_name AS seller_name,
       seller.avatar_url   AS seller_avatar_url
FROM transactions
JOIN inventory_items ON inventory_items.id = transactions.item_id
JOIN users AS buyer  ON buyer.id = transactions.buyer_id
JOIN users AS seller ON seller.id = transactions.seller_id
WHERE transactions.id = $1;

-- Totais do usuário. Compras canceladas/falhas não contam (foram estornadas).
-- A taxa sai da parte do vendedor.
-- name: GetUserTransactionSummary :one
SELECT
    COUNT(*) FILTER (WHERE buyer_id = @user_id AND status NOT IN ('cancelled', 'failed'))                            AS purchases,
    COALESCE(SUM(price_cents) FILTER (WHERE buyer_id = @user_id AND status NOT IN ('cancelled', 'failed')), 0)::bigint AS spent_cents,
    COUNT(*) FILTER (WHERE seller_id = @user_id AND status = 'completed')                                              AS sales,
    COALESCE(SUM(price_cents - fee_cents) FILTER (WHERE seller_id = @user_id AND status = 'completed'), 0)::bigint      AS earned_cents,
    COALESCE(SUM(price_cents - fee_cents) FILTER (WHERE seller_id = @user_id AND status IN ('pending', 'sent', 'on_hold')), 0)::bigint AS pending_earnings_cents,
    COALESCE(SUM(fee_cents) FILTER (WHERE seller_id = @user_id AND status = 'completed'), 0)::bigint                   AS fees_paid_cents
FROM transactions
WHERE buyer_id = @user_id OR seller_id = @user_id;
