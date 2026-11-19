-- name: GetInventoryItem :one
SELECT * FROM inventory_items
WHERE id = $1;

-- name: CreateListing :one
INSERT INTO listings (item_id, seller_id, price_cents)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetListing :one
SELECT * FROM listings
WHERE id = $1;

-- Só cancela se ainda estiver ativa e for do vendedor. Sem linha de volta = não pôde.
-- name: CancelListing :one
UPDATE listings
SET status = 'cancelled', updated_at = now()
WHERE id = @id AND seller_id = @seller_id AND status = 'active'
RETURNING *;

-- name: ListListingsBySeller :many
SELECT sqlc.embed(listings), sqlc.embed(inventory_items)
FROM listings
JOIN inventory_items ON inventory_items.id = listings.item_id
WHERE listings.seller_id = @seller_id
  AND (sqlc.narg(status)::text IS NULL OR listings.status = sqlc.narg(status)::text)
ORDER BY listings.created_at DESC
LIMIT 200;

-- Chamada no sync do inventário: cancela listings cujo item saiu do inventário,
-- mudou de dono ou deixou de ser negociável.
-- name: CancelInvalidListings :execrows
UPDATE listings
SET status = 'cancelled', updated_at = now()
FROM inventory_items
WHERE listings.item_id = inventory_items.id
  AND listings.status = 'active'
  AND (inventory_items.owner_id = @owner_id OR listings.seller_id = @owner_id)
  AND (inventory_items.removed_at IS NOT NULL
       OR inventory_items.owner_id <> listings.seller_id
       OR NOT inventory_items.tradable);
