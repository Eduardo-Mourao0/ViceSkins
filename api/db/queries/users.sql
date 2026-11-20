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

-- Usuários criados pela seed de demonstração (db/seed/dev_seed.sql).
-- name: ListDemoUsers :many
SELECT * FROM users
WHERE steam_id LIKE 'demo-%'
ORDER BY id;
