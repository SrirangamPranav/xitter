package auth

import (
	"context"

	"github.com/monis/twitter-backend-go/internal/db/sqlc"
)

type contextKey string

const (
	userContextKey    contextKey = "current_user"
	sessionContextKey contextKey = "current_session"
)

// WithUser adds the authenticated user to the context
func WithUser(ctx context.Context, user *sqlc.User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// GetUser retrieves the authenticated user from context, returning nil if unauthenticated
func GetUser(ctx context.Context) *sqlc.User {
	user, ok := ctx.Value(userContextKey).(*sqlc.User)
	if !ok {
		return nil
	}
	return user
}

// WithSession adds session metadata to the context
func WithSession(ctx context.Context, session *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey, session)
}

// GetSession retrieves session metadata from context
func GetSession(ctx context.Context) *Session {
	sess, ok := ctx.Value(sessionContextKey).(*Session)
	if !ok {
		return nil
	}
	return sess
}
