package graph

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/monis/twitter-backend-go/internal/auth"
	"github.com/monis/twitter-backend-go/internal/cache"
	"github.com/monis/twitter-backend-go/internal/db/sqlc"
	"github.com/monis/twitter-backend-go/internal/graph/dataloader"
	"github.com/monis/twitter-backend-go/internal/graph/generated"
	"github.com/monis/twitter-backend-go/internal/graph/model"
)

// Author resolves the tweet's author using DataLoader to avoid N+1 queries
func (r *tweetResolver) Author(ctx context.Context, obj *model.Tweet) (*model.User, error) {
	if obj.Author != nil {
		return obj.Author, nil
	}

	if obj.AuthorID == "" {
		return nil, fmt.Errorf("missing author id for tweet %s", obj.ID)
	}

	// 1. Try DataLoader for batching
	if loader := dataloader.For(ctx); loader != nil {
		return loader.UserLoader.Load(ctx, obj.AuthorID)
	}

	// 2. Fallback to LRU Cache + DB
	userKey := cache.UserKey(obj.AuthorID)
	return cache.GetOrSet(r.Cache, ctx, userKey, r.Config.UserCacheTTL, func() (*model.User, error) {
		var uUUID pgtype.UUID
		if err := uUUID.Scan(obj.AuthorID); err != nil {
			return nil, err
		}
		u, err := r.Store.GetUserByID(ctx, uUUID)
		if err != nil {
			return nil, err
		}
		return &model.User{
			ID:          sqlc.UUIDToString(u.ID),
			Email:       u.Email,
			Handle:      u.Handle,
			DisplayName: u.DisplayName,
			AvatarURL:   u.AvatarUrl,
			Bio:         u.Bio,
			CreatedAt:   sqlc.TimeToTime(u.CreatedAt).Format("2006-01-02T15:04:05Z07:00"),
		}, nil
	})
}

// ParentTweet resolves parent tweet for threads/replies
func (r *tweetResolver) ParentTweet(ctx context.Context, obj *model.Tweet) (*model.Tweet, error) {
	if obj.ParentTweetID == nil || *obj.ParentTweetID == "" {
		return nil, nil
	}

	parentID := *obj.ParentTweetID
	cacheKey := cache.TweetKey(parentID)
	return cache.GetOrSet(r.Cache, ctx, cacheKey, r.Config.CacheDefaultTTL, func() (*model.Tweet, error) {
		var tUUID pgtype.UUID
		if err := tUUID.Scan(parentID); err != nil {
			return nil, err
		}
		t, err := r.Store.GetTweetByID(ctx, tUUID)
		if err != nil {
			return nil, err
		}
		return toModelTweet(&t), nil
	})
}

// HasLiked resolves whether current viewer has liked this tweet using DataLoader
func (r *tweetResolver) HasLiked(ctx context.Context, obj *model.Tweet) (bool, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return false, nil
	}

	if loader := dataloader.For(ctx); loader != nil {
		return loader.LikeStatusLoader.Load(ctx, obj.ID)
	}

	var userUUID, tweetUUID pgtype.UUID
	userUUID.Scan(sqlc.UUIDToString(currentUser.ID))
	tweetUUID.Scan(obj.ID)

	return r.Store.HasUserLikedTweet(ctx, sqlc.HasUserLikedTweetParams{
		UserID:  userUUID,
		TweetID: tweetUUID,
	})
}

type tweetResolver struct{ *Resolver }

func (r *Resolver) Tweet() generated.TweetResolver { return &tweetResolver{r} }

// Helper converting sqlc.Tweet to model.Tweet
func toModelTweet(t *sqlc.Tweet) *model.Tweet {
	idStr := sqlc.UUIDToString(t.ID)
	authorIDStr := sqlc.UUIDToString(t.UserID)
	var parentID *string
	if t.ParentTweetID.Valid {
		pStr := sqlc.UUIDToString(t.ParentTweetID)
		parentID = &pStr
	}

	return &model.Tweet{
		ID:            idStr,
		AuthorID:      authorIDStr,
		Content:       t.Content,
		ParentTweetID: parentID,
		LikesCount:    int(t.LikesCount),
		RepliesCount:  int(t.RepliesCount),
		RetweetsCount: int(t.RetweetsCount),
		CreatedAt:     sqlc.TimeToTime(t.CreatedAt).Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:     sqlc.TimeToTime(t.UpdatedAt).Format("2006-01-02T15:04:05Z07:00"),
	}
}
