-- Reserva o refresh de forma atômica: só atualiza se o cooldown já passou.
-- Duas requisições simultâneas: só uma recebe a linha de volta.
-- name: ClaimInventoryRefresh :one
UPDATE users
SET inventory_refresh_at = @now::timestamptz
WHERE id = @user_id
  AND (inventory_refresh_at IS NULL OR inventory_refresh_at < @cutoff::timestamptz)
RETURNING id;

-- name: UpsertInventoryItem :exec
INSERT INTO inventory_items (
    owner_id, asset_id, class_id, instance_id, market_hash_name, name, item_type,
    weapon, rarity, rarity_color, exterior, float_value, paint_seed, icon_url,
    stickers, tradable, marketable
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
)
ON CONFLICT (asset_id) DO UPDATE
SET owner_id         = EXCLUDED.owner_id,
    market_hash_name = EXCLUDED.market_hash_name,
    name             = EXCLUDED.name,
    item_type        = EXCLUDED.item_type,
    weapon           = EXCLUDED.weapon,
    rarity           = EXCLUDED.rarity,
    rarity_color     = EXCLUDED.rarity_color,
    exterior         = EXCLUDED.exterior,
    float_value      = EXCLUDED.float_value,
    paint_seed       = EXCLUDED.paint_seed,
    icon_url         = EXCLUDED.icon_url,
    stickers         = EXCLUDED.stickers,
    tradable         = EXCLUDED.tradable,
    marketable       = EXCLUDED.marketable,
    removed_at       = NULL,
    updated_at       = now();

-- name: MarkMissingInventoryItemsRemoved :execrows
UPDATE inventory_items
SET removed_at = now(),
    updated_at = now()
WHERE owner_id = @owner_id
  AND removed_at IS NULL
  AND NOT (asset_id = ANY (@present_asset_ids::text[]));

-- name: SetInventorySynced :exec
UPDATE users
SET inventory_synced_at = now()
WHERE id = $1;

-- name: ListInventoryByOwner :many
SELECT sqlc.embed(inventory_items),
       listings.id          AS listing_id,
       listings.price_cents AS listing_price_cents
FROM inventory_items
LEFT JOIN listings
       ON listings.item_id = inventory_items.id AND listings.status = 'active'
WHERE inventory_items.owner_id = $1
  AND inventory_items.removed_at IS NULL
ORDER BY inventory_items.tradable DESC, inventory_items.market_hash_name, inventory_items.id;
