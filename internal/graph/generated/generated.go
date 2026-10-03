package generated

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/monis/twitter-backend-go/internal/graph/model"
	"github.com/vektah/gqlparser/v2/ast"
)

type Config struct {
	Resolvers ResolverRoot
}

type ResolverRoot interface {
	Mutation() MutationResolver
	Query() QueryResolver
	Subscription() SubscriptionResolver
	Tweet() TweetResolver
	User() UserResolver
}

type MutationResolver interface {
	CreateTweet(ctx context.Context, input model.CreateTweetInput) (*model.Tweet, error)
	DeleteTweet(ctx context.Context, id string) (bool, error)
	LikeTweet(ctx context.Context, tweetID string) (*model.Tweet, error)
	UnlikeTweet(ctx context.Context, tweetID string) (*model.Tweet, error)
	FollowUser(ctx context.Context, userID string) (bool, error)
	UnfollowUser(ctx context.Context, userID string) (bool, error)
	UpdateProfile(ctx context.Context, input model.UpdateProfileInput) (*model.User, error)
	Logout(ctx context.Context) (bool, error)
	RevokeAllSessions(ctx context.Context) (bool, error)
}

type QueryResolver interface {
	Me(ctx context.Context) (*model.User, error)
	User(ctx context.Context, id *string, handle *string) (*model.User, error)
	Tweet(ctx context.Context, id string) (*model.Tweet, error)
	Feed(ctx context.Context, limit *int, cursor *string) (*model.TweetConnection, error)
	UserTweets(ctx context.Context, userID string, limit *int, cursor *string) (*model.TweetConnection, error)
	TweetReplies(ctx context.Context, tweetID string, limit *int, cursor *string) (*model.TweetConnection, error)
	SearchUsers(ctx context.Context, query string, limit *int) ([]*model.User, error)
}

type SubscriptionResolver interface {
	TweetAdded(ctx context.Context, authorID *string) (<-chan *model.Tweet, error)
	FeedUpdated(ctx context.Context) (<-chan *model.Tweet, error)
	TweetLiked(ctx context.Context, tweetID string) (<-chan *model.TweetLikeEvent, error)
}

type TweetResolver interface {
	Author(ctx context.Context, obj *model.Tweet) (*model.User, error)
	ParentTweet(ctx context.Context, obj *model.Tweet) (*model.Tweet, error)
	HasLiked(ctx context.Context, obj *model.Tweet) (bool, error)
}

type UserResolver interface {
	FollowersCount(ctx context.Context, obj *model.User) (int, error)
	FollowingCount(ctx context.Context, obj *model.User) (int, error)
	IsFollowing(ctx context.Context, obj *model.User) (bool, error)
}

type executableSchema struct {
	resolvers ResolverRoot
}

func NewExecutableSchema(cfg Config) graphql.ExecutableSchema {
	return &executableSchema{
		resolvers: cfg.Resolvers,
	}
}

func (e *executableSchema) Schema() *ast.Schema {
	return &ast.Schema{}
}

func (e *executableSchema) Complexity(typeName, field string, childComplexity int, args map[string]interface{}) (int, bool) {
	return 1, true
}

func (e *executableSchema) Exec(ctx context.Context) graphql.ResponseHandler {
	return func(ctx context.Context) *graphql.Response {
		return &graphql.Response{
			Data: []byte(`{}`),
		}
	}
}
