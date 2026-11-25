-- name: UpsertUserBySteamID :one
INSERT INTO users (steam_id, display_name, avatar_url)
VALUES ($1, $2, $3)
ON CONFLICT (steam_id) DO UPDATE
SET display_name = EXCLUDED.display_name,
    avatar_url   = EXCLUDED.avatar_url,
    updated_at   = now()
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1;

-- name: SetTradeURL :one
UPDATE users
SET trade_url = @trade_url, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetSteamAPIKey :one
UPDATE users
SET steam_api_key_enc = @key_enc, steam_api_key_last4 = @key_last4, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: GetSteamAPIKey :one
SELECT steam_api_key_enc FROM users
WHERE id = $1;
