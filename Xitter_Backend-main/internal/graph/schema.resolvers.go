package graph

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/monis/twitter-backend-go/internal/auth"
	"github.com/monis/twitter-backend-go/internal/cache"
	"github.com/monis/twitter-backend-go/internal/db/sqlc"
	"github.com/monis/twitter-backend-go/internal/graph/generated"
	"github.com/monis/twitter-backend-go/internal/graph/model"
	"github.com/monis/twitter-backend-go/internal/pubsub"
)

// Me returns the currently authenticated user
func (r *queryResolver) Me(ctx context.Context) (*model.User, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return nil, nil
	}
	return toModelUser(currentUser), nil
}

// User fetches a user profile by ID or handle with LRU cache-aside
func (r *queryResolver) User(ctx context.Context, id *string, handle *string) (*model.User, error) {
	if id != nil && *id != "" {
		cacheKey := cache.UserKey(*id)
		return cache.GetOrSet(r.Cache, ctx, cacheKey, r.Config.UserCacheTTL, func() (*model.User, error) {
			var uUUID pgtype.UUID
			if err := uUUID.Scan(*id); err != nil {
				return nil, fmt.Errorf("invalid user id format: %w", err)
			}
			user, err := r.Store.GetUserByID(ctx, uUUID)
			if err != nil {
				return nil, fmt.Errorf("user not found: %w", err)
			}
			return toModelUser(&user), nil
		})
	}

	if handle != nil && *handle != "" {
		cacheKey := cache.UserHandleKey(*handle)
		return cache.GetOrSet(r.Cache, ctx, cacheKey, r.Config.UserCacheTTL, func() (*model.User, error) {
			user, err := r.Store.GetUserByHandle(ctx, *handle)
			if err != nil {
				return nil, fmt.Errorf("user not found with handle %s: %w", *handle, err)
			}
			return toModelUser(&user), nil
		})
	}

	return nil, errors.New("must specify either id or handle")
}

// Tweet fetches a single tweet by ID with LRU cache-aside
func (r *queryResolver) Tweet(ctx context.Context, id string) (*model.Tweet, error) {
	cacheKey := cache.TweetKey(id)
	return cache.GetOrSet(r.Cache, ctx, cacheKey, r.Config.CacheDefaultTTL, func() (*model.Tweet, error) {
		var tUUID pgtype.UUID
		if err := tUUID.Scan(id); err != nil {
			return nil, fmt.Errorf("invalid tweet id: %w", err)
		}
		tweet, err := r.Store.GetTweetByID(ctx, tUUID)
		if err != nil {
			return nil, fmt.Errorf("tweet not found: %w", err)
		}
		return toModelTweet(&tweet), nil
	})
}

// Feed returns a cursor-paginated timeline with LRU caching
func (r *queryResolver) Feed(ctx context.Context, limit *int, cursor *string) (*model.TweetConnection, error) {
	limitVal := 20
	if limit != nil && *limit > 0 && *limit <= 100 {
		limitVal = *limit
	}

	cursorStr := ""
	var cursorTime pgtype.Timestamptz
	if cursor != nil && *cursor != "" {
		cursorStr = *cursor
		decoded, err := base64.StdEncoding.DecodeString(*cursor)
		if err == nil {
			if t, parseErr := time.Parse(time.RFC3339Nano, string(decoded)); parseErr == nil {
				cursorTime = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}
	}

	currentUser := auth.GetUser(ctx)
	var cacheKey string
	if currentUser != nil {
		cacheKey = cache.FeedKey(sqlc.UUIDToString(currentUser.ID), cursorStr)
	} else {
		cacheKey = cache.GlobalFeedKey(cursorStr)
	}

	return cache.GetOrSet(r.Cache, ctx, cacheKey, r.Config.FeedCacheTTL, func() (*model.TweetConnection, error) {
		var tweets []sqlc.Tweet
		var err error

		// Fetch limitVal + 1 to check if there is a next page
		fetchCount := int32(limitVal + 1)

		if currentUser != nil {
			tweets, err = r.Store.GetHomeFeed(ctx, sqlc.GetHomeFeedParams{
				UserID:  currentUser.ID,
				Dollar2: cursorTime,
				Limit:   fetchCount,
			})
		} else {
			tweets, err = r.Store.GetGlobalFeed(ctx, sqlc.GetGlobalFeedParams{
				Dollar1: cursorTime,
				Limit:   fetchCount,
			})
		}

		if err != nil {
			return nil, fmt.Errorf("failed to fetch timeline: %w", err)
		}

		hasNextPage := len(tweets) > limitVal
		if hasNextPage {
			tweets = tweets[:limitVal]
		}

		edges := make([]*model.TweetEdge, len(tweets))
		var endCursor *string

		for i, t := range tweets {
			tweetModel := toModelTweet(&t)
			encodedCursor := base64.StdEncoding.EncodeToString([]byte(sqlc.TimeToTime(t.CreatedAt).Format(time.RFC3339Nano)))
			edges[i] = &model.TweetEdge{
				Node:   tweetModel,
				Cursor: encodedCursor,
			}
			if i == len(tweets)-1 {
				endCursor = &encodedCursor
			}
		}

		return &model.TweetConnection{
			Edges: edges,
			PageInfo: &model.PageInfo{
				HasNextPage: hasNextPage,
				EndCursor:   endCursor,
			},
		}, nil
	})
}

// UserTweets returns tweets created by a specific user with cursor pagination
func (r *queryResolver) UserTweets(ctx context.Context, userID string, limit *int, cursor *string) (*model.TweetConnection, error) {
	limitVal := 20
	if limit != nil && *limit > 0 && *limit <= 100 {
		limitVal = *limit
	}

	var cursorTime pgtype.Timestamptz
	if cursor != nil && *cursor != "" {
		decoded, err := base64.StdEncoding.DecodeString(*cursor)
		if err == nil {
			if t, parseErr := time.Parse(time.RFC3339Nano, string(decoded)); parseErr == nil {
				cursorTime = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}
	}

	var uUUID pgtype.UUID
	if err := uUUID.Scan(userID); err != nil {
		return nil, fmt.Errorf("invalid user id: %w", err)
	}

	tweets, err := r.Store.GetUserTweets(ctx, sqlc.GetUserTweetsParams{
		UserID:  uUUID,
		Dollar2: cursorTime,
		Limit:   int32(limitVal + 1),
	})
	if err != nil {
		return nil, err
	}

	hasNextPage := len(tweets) > limitVal
	if hasNextPage {
		tweets = tweets[:limitVal]
	}

	edges := make([]*model.TweetEdge, len(tweets))
	var endCursor *string

	for i, t := range tweets {
		tweetModel := toModelTweet(&t)
		encodedCursor := base64.StdEncoding.EncodeToString([]byte(sqlc.TimeToTime(t.CreatedAt).Format(time.RFC3339Nano)))
		edges[i] = &model.TweetEdge{
			Node:   tweetModel,
			Cursor: encodedCursor,
		}
		if i == len(tweets)-1 {
			endCursor = &encodedCursor
		}
	}

	return &model.TweetConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage: hasNextPage,
			EndCursor:   endCursor,
		},
	}, nil
}

// TweetReplies returns child replies for a tweet thread
func (r *queryResolver) TweetReplies(ctx context.Context, tweetID string, limit *int, cursor *string) (*model.TweetConnection, error) {
	limitVal := 20
	if limit != nil && *limit > 0 && *limit <= 100 {
		limitVal = *limit
	}

	var cursorTime pgtype.Timestamptz
	if cursor != nil && *cursor != "" {
		decoded, err := base64.StdEncoding.DecodeString(*cursor)
		if err == nil {
			if t, parseErr := time.Parse(time.RFC3339Nano, string(decoded)); parseErr == nil {
				cursorTime = pgtype.Timestamptz{Time: t, Valid: true}
			}
		}
	}

	var tUUID pgtype.UUID
	if err := tUUID.Scan(tweetID); err != nil {
		return nil, fmt.Errorf("invalid tweet id: %w", err)
	}

	replies, err := r.Store.GetTweetReplies(ctx, sqlc.GetTweetRepliesParams{
		ParentTweetID: tUUID,
		Dollar2:       cursorTime,
		Limit:         int32(limitVal + 1),
	})
	if err != nil {
		return nil, err
	}

	hasNextPage := len(replies) > limitVal
	if hasNextPage {
		replies = replies[:limitVal]
	}

	edges := make([]*model.TweetEdge, len(replies))
	var endCursor *string

	for i, t := range replies {
		tweetModel := toModelTweet(&t)
		encodedCursor := base64.StdEncoding.EncodeToString([]byte(sqlc.TimeToTime(t.CreatedAt).Format(time.RFC3339Nano)))
		edges[i] = &model.TweetEdge{
			Node:   tweetModel,
			Cursor: encodedCursor,
		}
		if i == len(replies)-1 {
			endCursor = &encodedCursor
		}
	}

	return &model.TweetConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage: hasNextPage,
			EndCursor:   endCursor,
		},
	}, nil
}

// SearchUsers finds users matching a query
func (r *queryResolver) SearchUsers(ctx context.Context, query string, limit *int) ([]*model.User, error) {
	limitVal := 10
	if limit != nil && *limit > 0 {
		limitVal = *limit
	}

	users, err := r.Store.SearchUsers(ctx, sqlc.SearchUsersParams{
		Dollar1: pgtype.Text{String: query, Valid: true},
		Limit:   int32(limitVal),
	})
	if err != nil {
		return nil, err
	}

	result := make([]*model.User, len(users))
	for i, u := range users {
		result[i] = toModelUser(&u)
	}
	return result, nil
}

// CreateTweet posts a new root tweet or reply and publishes a WebSocket event
func (r *mutationResolver) CreateTweet(ctx context.Context, input model.CreateTweetInput) (*model.Tweet, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return nil, auth.ErrUnauthorized
	}

	if len(input.Content) == 0 || len(input.Content) > 280 {
		return nil, errors.New("tweet content must be between 1 and 280 characters")
	}

	var parentUUID pgtype.UUID
	if input.ParentTweetID != nil && *input.ParentTweetID != "" {
		if err := parentUUID.Scan(*input.ParentTweetID); err != nil {
			return nil, fmt.Errorf("invalid parent tweet id: %w", err)
		}
	}

	tweet, err := r.Store.CreateTweet(ctx, sqlc.CreateTweetParams{
		UserID:        currentUser.ID,
		Content:       input.Content,
		ParentTweetID: parentUUID,
		RetweetOfID:   pgtype.UUID{Valid: false},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tweet: %w", err)
	}

	// If it was a reply, increment parent tweet replies_count
	if parentUUID.Valid {
		_ = r.Store.IncrementTweetReplies(ctx, parentUUID)
		_ = r.Cache.Invalidate(ctx, cache.TweetKey(sqlc.UUIDToString(parentUUID)))
	}

	// Invalidate feed caches in LRU Redis
	authorIDStr := sqlc.UUIDToString(currentUser.ID)
	_ = r.Cache.InvalidatePattern(ctx, fmt.Sprintf("cache:feed:%s:*", authorIDStr))
	_ = r.Cache.InvalidatePattern(ctx, "cache:feed:global:*")

	modelTweet := toModelTweet(&tweet)
	modelTweet.Author = toModelUser(currentUser)

	// Broadcast to Redis PubSub for real-time WebSocket subscribers
	go func() {
		bgCtx := context.Background()
		_ = r.PubSub.Publish(bgCtx, pubsub.TweetChannel(""), modelTweet)
		_ = r.PubSub.Publish(bgCtx, pubsub.TweetChannel(authorIDStr), modelTweet)
	}()

	return modelTweet, nil
}

// DeleteTweet removes a tweet and busts its cache
func (r *mutationResolver) DeleteTweet(ctx context.Context, id string) (bool, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return false, auth.ErrUnauthorized
	}

	var tUUID pgtype.UUID
	if err := tUUID.Scan(id); err != nil {
		return false, fmt.Errorf("invalid tweet id: %w", err)
	}

	if err := r.Store.DeleteTweet(ctx, sqlc.DeleteTweetParams{
		ID:     tUUID,
		UserID: currentUser.ID,
	}); err != nil {
		return false, fmt.Errorf("failed to delete tweet: %w", err)
	}

	// Evict from LRU Cache
	_ = r.Cache.Invalidate(ctx, cache.TweetKey(id))
	authorIDStr := sqlc.UUIDToString(currentUser.ID)
	_ = r.Cache.InvalidatePattern(ctx, fmt.Sprintf("cache:feed:%s:*", authorIDStr))

	return true, nil
}

// LikeTweet records a like and broadcasts real-time like count
func (r *mutationResolver) LikeTweet(ctx context.Context, tweetID string) (*model.Tweet, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return nil, auth.ErrUnauthorized
	}

	var tUUID pgtype.UUID
	if err := tUUID.Scan(tweetID); err != nil {
		return nil, fmt.Errorf("invalid tweet id: %w", err)
	}

	if err := r.Store.LikeTweet(ctx, sqlc.LikeTweetParams{
		UserID:  currentUser.ID,
		TweetID: tUUID,
	}); err != nil {
		return nil, fmt.Errorf("failed to like tweet: %w", err)
	}

	_ = r.Store.IncrementTweetLikes(ctx, tUUID)

	// Invalidate tweet cache
	_ = r.Cache.Invalidate(ctx, cache.TweetKey(tweetID))

	// Fetch updated tweet
	updatedTweet, err := r.Store.GetTweetByID(ctx, tUUID)
	if err != nil {
		return nil, err
	}

	mTweet := toModelTweet(&updatedTweet)
	mTweet.HasLiked = true

	// Publish WebSocket event
	go func() {
		bgCtx := context.Background()
		_ = r.PubSub.Publish(bgCtx, pubsub.TweetLikeChannel(tweetID), model.TweetLikeEvent{
			TweetID:    tweetID,
			UserID:     sqlc.UUIDToString(currentUser.ID),
			LikesCount: int(updatedTweet.LikesCount),
		})
	}()

	return mTweet, nil
}

// UnlikeTweet removes a like
func (r *mutationResolver) UnlikeTweet(ctx context.Context, tweetID string) (*model.Tweet, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return nil, auth.ErrUnauthorized
	}

	var tUUID pgtype.UUID
	if err := tUUID.Scan(tweetID); err != nil {
		return nil, fmt.Errorf("invalid tweet id: %w", err)
	}

	if err := r.Store.UnlikeTweet(ctx, sqlc.UnlikeTweetParams{
		UserID:  currentUser.ID,
		TweetID: tUUID,
	}); err != nil {
		return nil, fmt.Errorf("failed to unlike tweet: %w", err)
	}

	_ = r.Store.DecrementTweetLikes(ctx, tUUID)
	_ = r.Cache.Invalidate(ctx, cache.TweetKey(tweetID))

	updatedTweet, err := r.Store.GetTweetByID(ctx, tUUID)
	if err != nil {
		return nil, err
	}

	mTweet := toModelTweet(&updatedTweet)
	mTweet.HasLiked = false
	return mTweet, nil
}

// FollowUser creates a social follow edge
func (r *mutationResolver) FollowUser(ctx context.Context, userID string) (bool, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return false, auth.ErrUnauthorized
	}

	var targetUUID pgtype.UUID
	if err := targetUUID.Scan(userID); err != nil {
		return false, fmt.Errorf("invalid target user id: %w", err)
	}

	if currentUser.ID == targetUUID {
		return false, errors.New("cannot follow yourself")
	}

	if err := r.Store.FollowUser(ctx, sqlc.FollowUserParams{
		FollowerID:  currentUser.ID,
		FollowingID: targetUUID,
	}); err != nil {
		return false, fmt.Errorf("failed to follow user: %w", err)
	}

	// Invalidate feed caches
	currentIDStr := sqlc.UUIDToString(currentUser.ID)
	_ = r.Cache.InvalidatePattern(ctx, fmt.Sprintf("cache:feed:%s:*", currentIDStr))

	return true, nil
}

// UnfollowUser removes a social follow edge
func (r *mutationResolver) UnfollowUser(ctx context.Context, userID string) (bool, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return false, auth.ErrUnauthorized
	}

	var targetUUID pgtype.UUID
	if err := targetUUID.Scan(userID); err != nil {
		return false, fmt.Errorf("invalid target user id: %w", err)
	}

	if err := r.Store.UnfollowUser(ctx, sqlc.UnfollowUserParams{
		FollowerID:  currentUser.ID,
		FollowingID: targetUUID,
	}); err != nil {
		return false, fmt.Errorf("failed to unfollow user: %w", err)
	}

	currentIDStr := sqlc.UUIDToString(currentUser.ID)
	_ = r.Cache.InvalidatePattern(ctx, fmt.Sprintf("cache:feed:%s:*", currentIDStr))

	return true, nil
}

// UpdateProfile updates user display name, bio, or avatar
func (r *mutationResolver) UpdateProfile(ctx context.Context, input model.UpdateProfileInput) (*model.User, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return nil, auth.ErrUnauthorized
	}

	var dName, bio, avatar pgtype.Text
	if input.DisplayName != nil {
		dName = pgtype.Text{String: *input.DisplayName, Valid: true}
	}
	if input.Bio != nil {
		bio = pgtype.Text{String: *input.Bio, Valid: true}
	}
	if input.AvatarURL != nil {
		avatar = pgtype.Text{String: *input.AvatarURL, Valid: true}
	}

	user, err := r.Store.UpdateUserProfile(ctx, sqlc.UpdateUserProfileParams{
		ID:          currentUser.ID,
		DisplayName: dName,
		Bio:         bio,
		AvatarUrl:   avatar,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Invalidate user cache in LRU Redis
	userIDStr := sqlc.UUIDToString(currentUser.ID)
	_ = r.Cache.Invalidate(ctx, cache.UserKey(userIDStr), cache.UserHandleKey(currentUser.Handle))

	return toModelUser(&user), nil
}

// Logout revokes the current session from the Session Redis instance
func (r *mutationResolver) Logout(ctx context.Context) (bool, error) {
	session := auth.GetSession(ctx)
	if session == nil {
		return true, nil
	}

	if err := r.SessionMgr.RevokeSession(ctx, session.ID); err != nil {
		return false, fmt.Errorf("failed to revoke session: %w", err)
	}

	return true, nil
}

// RevokeAllSessions invalidates all sessions across all devices for the current user
func (r *mutationResolver) RevokeAllSessions(ctx context.Context) (bool, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return false, auth.ErrUnauthorized
	}

	userIDStr := sqlc.UUIDToString(currentUser.ID)
	if err := r.SessionMgr.RevokeAllUserSessions(ctx, userIDStr); err != nil {
		return false, fmt.Errorf("failed to revoke all sessions: %w", err)
	}

	return true, nil
}

type queryResolver struct{ *Resolver }

func (r *Resolver) Query() generated.QueryResolver { return &queryResolver{r} }

type mutationResolver struct{ *Resolver }

func (r *Resolver) Mutation() generated.MutationResolver { return &mutationResolver{r} }
