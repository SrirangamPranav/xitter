-- name: FollowUser :exec
INSERT INTO follows (follower_id, following_id)
VALUES ($1, $2)
ON CONFLICT (follower_id, following_id) DO NOTHING;

-- name: UnfollowUser :exec
DELETE FROM follows
WHERE follower_id = $1 AND following_id = $2;

-- name: IsFollowing :one
SELECT EXISTS (
    SELECT 1 FROM follows
    WHERE follower_id = $1 AND following_id = $2
) AS is_following;

-- name: GetFollowersCount :one
SELECT COUNT(*) FROM follows
WHERE following_id = $1;

-- name: GetFollowingCount :one
SELECT COUNT(*) FROM follows
WHERE follower_id = $1;

-- name: GetFollowers :many
SELECT u.id, u.google_id, u.email, u.handle, u.display_name, u.avatar_url, u.bio, u.created_at, u.updated_at
FROM users u
JOIN follows f ON f.follower_id = u.id
WHERE f.following_id = $1
ORDER BY f.created_at DESC
LIMIT $2;

-- name: GetFollowing :many
SELECT u.id, u.google_id, u.email, u.handle, u.display_name, u.avatar_url, u.bio, u.created_at, u.updated_at
FROM users u
JOIN follows f ON f.following_id = u.id
WHERE f.follower_id = $1
ORDER BY f.created_at DESC
LIMIT $2;
