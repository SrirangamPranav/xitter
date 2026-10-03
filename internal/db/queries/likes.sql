-- name: LikeTweet :exec
INSERT INTO likes (user_id, tweet_id)
VALUES ($1, $2)
ON CONFLICT (user_id, tweet_id) DO NOTHING;

-- name: UnlikeTweet :exec
DELETE FROM likes
WHERE user_id = $1 AND tweet_id = $2;

-- name: HasUserLikedTweet :one
SELECT EXISTS (
    SELECT 1 FROM likes
    WHERE user_id = $1 AND tweet_id = $2
) AS has_liked;

-- name: GetUserLikedTweetIDs :many
SELECT tweet_id FROM likes
WHERE user_id = $1 AND tweet_id = ANY($2::uuid[]);

-- name: GetTweetLikesCount :one
SELECT COUNT(*) FROM likes
WHERE tweet_id = $1;
