package graph

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/monis/twitter-backend-go/internal/auth"
	"github.com/monis/twitter-backend-go/internal/db/sqlc"
	"github.com/monis/twitter-backend-go/internal/graph/generated"
	"github.com/monis/twitter-backend-go/internal/graph/model"
)

// FollowersCount returns the total number of followers for a user
func (r *userResolver) FollowersCount(ctx context.Context, obj *model.User) (int, error) {
	var uUUID pgtype.UUID
	if err := uUUID.Scan(obj.ID); err != nil {
		return 0, err
	}
	count, err := r.Store.GetFollowersCount(ctx, uUUID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// FollowingCount returns the total number of accounts followed by a user
func (r *userResolver) FollowingCount(ctx context.Context, obj *model.User) (int, error) {
	var uUUID pgtype.UUID
	if err := uUUID.Scan(obj.ID); err != nil {
		return 0, err
	}
	count, err := r.Store.GetFollowingCount(ctx, uUUID)
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// IsFollowing checks if current logged-in user follows this user
func (r *userResolver) IsFollowing(ctx context.Context, obj *model.User) (bool, error) {
	currentUser := auth.GetUser(ctx)
	if currentUser == nil {
		return false, nil
	}

	var followerUUID, followingUUID pgtype.UUID
	followerUUID.Scan(sqlc.UUIDToString(currentUser.ID))
	followingUUID.Scan(obj.ID)

	if followerUUID == followingUUID {
		return false, nil
	}

	return r.Store.IsFollowing(ctx, sqlc.IsFollowingParams{
		FollowerID:  followerUUID,
		FollowingID: followingUUID,
	})
}

type userResolver struct{ *Resolver }

func (r *Resolver) User() generated.UserResolver { return &userResolver{r} }

// Helper converting sqlc.User to model.User
func toModelUser(u *sqlc.User) *model.User {
	return &model.User{
		ID:          sqlc.UUIDToString(u.ID),
		Email:       u.Email,
		Handle:      u.Handle,
		DisplayName: u.DisplayName,
		AvatarURL:   u.AvatarUrl,
		Bio:         u.Bio,
		CreatedAt:   sqlc.TimeToTime(u.CreatedAt).Format("2006-01-02T15:04:05Z07:00"),
	}
}
