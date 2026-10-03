-- name: CreateTweet :one
INSERT INTO tweets (user_id, content, parent_tweet_id, retweet_of_id)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, content, parent_tweet_id, retweet_of_id, likes_count, replies_count, retweets_count, created_at, updated_at;

-- name: GetTweetByID :one
SELECT id, user_id, content, parent_tweet_id, retweet_of_id, likes_count, replies_count, retweets_count, created_at, updated_at
FROM tweets
WHERE id = $1 LIMIT 1;

-- name: GetTweetsByIDs :many
SELECT id, user_id, content, parent_tweet_id, retweet_of_id, likes_count, replies_count, retweets_count, created_at, updated_at
FROM tweets
WHERE id = ANY($1::uuid[])
ORDER BY created_at DESC;

-- name: DeleteTweet :exec
DELETE FROM tweets
WHERE id = $1 AND user_id = $2;

-- name: GetUserTweets :many
SELECT id, user_id, content, parent_tweet_id, retweet_of_id, likes_count, replies_count, retweets_count, created_at, updated_at
FROM tweets
WHERE user_id = $1
  AND ($2::timestamptz IS NULL OR created_at < $2)
ORDER BY created_at DESC
LIMIT $3;

-- name: GetHomeFeed :many
-- Optimized timeline query fetching tweets from users the current user follows + user's own tweets
SELECT t.id, t.user_id, t.content, t.parent_tweet_id, t.retweet_of_id, t.likes_count, t.replies_count, t.retweets_count, t.created_at, t.updated_at
FROM tweets t
WHERE (
    t.user_id = $1
    OR t.user_id IN (
        SELECT following_id FROM follows WHERE follower_id = $1
    )
)
AND ($2::timestamptz IS NULL OR t.created_at < $2)
AND t.parent_tweet_id IS NULL -- Root tweets and retweets in main feed
ORDER BY t.created_at DESC
LIMIT $3;

-- name: GetGlobalFeed :many
SELECT id, user_id, content, parent_tweet_id, retweet_of_id, likes_count, replies_count, retweets_count, created_at, updated_at
FROM tweets
WHERE parent_tweet_id IS NULL
  AND ($1::timestamptz IS NULL OR created_at < $1)
ORDER BY created_at DESC
LIMIT $2;

-- name: GetTweetReplies :many
SELECT id, user_id, content, parent_tweet_id, retweet_of_id, likes_count, replies_count, retweets_count, created_at, updated_at
FROM tweets
WHERE parent_tweet_id = $1
  AND ($2::timestamptz IS NULL OR created_at < $2)
ORDER BY created_at ASC
LIMIT $3;

-- name: IncrementTweetLikes :exec
UPDATE tweets
SET likes_count = likes_count + 1, updated_at = NOW()
WHERE id = $1;

-- name: DecrementTweetLikes :exec
UPDATE tweets
SET likes_count = GREATEST(0, likes_count - 1), updated_at = NOW()
WHERE id = $1;

-- name: IncrementTweetReplies :exec
UPDATE tweets
SET replies_count = replies_count + 1, updated_at = NOW()
WHERE id = $1;

-- name: DecrementTweetReplies :exec
UPDATE tweets
SET replies_count = GREATEST(0, replies_count - 1), updated_at = NOW()
WHERE id = $1;

-- name: IncrementTweetRetweets :exec
UPDATE tweets
SET retweets_count = retweets_count + 1, updated_at = NOW()
WHERE id = $1;

-- name: DecrementTweetRetweets :exec
UPDATE tweets
SET retweets_count = GREATEST(0, retweets_count - 1), updated_at = NOW()
WHERE id = $1;
