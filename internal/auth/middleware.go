package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/monis/twitter-backend-go/internal/config"
	"github.com/monis/twitter-backend-go/internal/db"
)

var (
	ErrUnauthorized = errors.New("authentication required: please log in")
)

type Middleware struct {
	sessionMgr *SessionManager
	store      *db.Store
	cfg        *config.Config
}

func NewMiddleware(sessionMgr *SessionManager, store *db.Store, cfg *config.Config) *Middleware {
	return &Middleware{
		sessionMgr: sessionMgr,
		store:      store,
		cfg:        cfg,
	}
}

// SessionMiddleware extracts session token from cookie or Authorization header,
// loads the user, and injects both into the request context.
func (m *Middleware) SessionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		token := ""

		// 1. Check HttpOnly cookie
		if cookie, err := r.Cookie(m.cfg.CookieName); err == nil && cookie.Value != "" {
			token = cookie.Value
		}

		// 2. Fallback to Authorization: Bearer <token> for API tools / mobile clients
		if token == "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if token == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Validate against Redis Session instance
		session, err := m.sessionMgr.GetSession(ctx, token)
		if err != nil || session == nil {
			// Invalid or expired session
			next.ServeHTTP(w, r)
			return
		}

		// Extend TTL asynchronously or touch
		go func() {
			_ = m.sessionMgr.TouchSession(context.Background(), token)
		}()

		// Parse user ID UUID
		var userUUID pgtype.UUID
		if err := userUUID.Scan(session.UserID); err != nil {
			next.ServeHTTP(w, r)
			return
		}

		user, err := m.store.GetUserByID(ctx, userUUID)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}

		// Inject user and session into context
		ctx = WithUser(ctx, &user)
		ctx = WithSession(ctx, session)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
