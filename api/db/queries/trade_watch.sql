-- ===== Verificação das trocas na Steam =====

-- Transações em andamento que não são verificadas há pelo menos o intervalo.
-- name: ListTradesToCheck :many
SELECT id FROM transactions
WHERE status IN ('pending', 'sent', 'on_hold')
  AND (last_checked_at IS NULL OR last_checked_at <= @cutoff::timestamptz)
ORDER BY last_checked_at NULLS FIRST, id
LIMIT @max_rows;

-- Encerradas com estorno há pouco tempo: uma oferta que ficou aberta ainda
-- pode ser aceita. Vigiadas por um período para detectar entrega tardia.
-- name: ListClosedTradesToWatch :many
SELECT id FROM transactions
WHERE status IN ('cancelled', 'failed')
  AND late_delivery_at IS NULL
  AND updated_at >= @since::timestamptz
  AND (last_checked_at IS NULL OR last_checked_at <= @cutoff::timestamptz)
ORDER BY last_checked_at NULLS FIRST, id
LIMIT @max_rows;

-- name: MarkTradeChecked :exec
UPDATE transactions SET last_checked_at = @now::timestamptz
WHERE id = @id;

-- name: MarkLateDelivery :exec
UPDATE transactions SET late_delivery_at = @now::timestamptz
WHERE id = @id AND late_delivery_at IS NULL;

-- Tudo que a verificação precisa saber: o item e as contas Steam das partes.
-- name: GetTradeContext :one
SELECT transactions.id, transactions.status, transactions.trade_offer_id, transactions.buyer_trade_url,
       transactions.seller_id, transactions.created_at,
       inventory_items.asset_id, inventory_items.market_hash_name, inventory_items.float_value, inventory_items.paint_seed,
       seller.steam_id AS seller_steam_id,
       buyer.steam_id  AS buyer_steam_id,
       seller.steam_api_key_enc AS seller_api_key_enc
FROM transactions
JOIN inventory_items ON inventory_items.id = transactions.item_id
JOIN users AS seller ON seller.id = transactions.seller_id
JOIN users AS buyer  ON buyer.id = transactions.buyer_id
WHERE transactions.id = $1;
