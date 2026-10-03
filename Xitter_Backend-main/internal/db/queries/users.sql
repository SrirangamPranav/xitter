-- name: GetUserByID :one
SELECT id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at
FROM users
WHERE id = $1 LIMIT 1;

-- name: GetUserByGoogleID :one
SELECT id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at
FROM users
WHERE google_id = $1 LIMIT 1;

-- name: GetUserByEmail :one
SELECT id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at
FROM users
WHERE email = $1 LIMIT 1;

-- name: GetUserByHandle :one
SELECT id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at
FROM users
WHERE handle = $1 LIMIT 1;

-- name: UpsertGoogleUser :one
INSERT INTO users (google_id, email, handle, display_name, avatar_url)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (email) DO UPDATE
SET google_id = EXCLUDED.google_id,
    display_name = CASE WHEN users.display_name = '' THEN EXCLUDED.display_name ELSE users.display_name END,
    avatar_url = CASE WHEN users.avatar_url = '' THEN EXCLUDED.avatar_url ELSE users.avatar_url END,
    updated_at = NOW()
RETURNING id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at;

-- name: CreateUser :one
INSERT INTO users (email, handle, display_name, avatar_url, bio)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at;

-- name: UpdateUserProfile :one
UPDATE users
SET display_name = COALESCE($2, display_name),
    bio = COALESCE($3, bio),
    avatar_url = COALESCE($4, avatar_url),
    updated_at = NOW()
WHERE id = $1
RETURNING id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at;

-- name: GetUsersByIDs :many
SELECT id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at
FROM users
WHERE id = ANY($1::uuid[]);

-- name: SearchUsers :many
SELECT id, google_id, email, handle, display_name, avatar_url, bio, created_at, updated_at
FROM users
WHERE handle ILIKE '%' || $1 || '%' OR display_name ILIKE '%' || $1 || '%'
ORDER BY created_at DESC
LIMIT $2;
