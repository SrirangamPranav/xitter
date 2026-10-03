package dataloader

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/monis/twitter-backend-go/internal/auth"
	"github.com/monis/twitter-backend-go/internal/db"
	"github.com/monis/twitter-backend-go/internal/db/sqlc"
	"github.com/monis/twitter-backend-go/internal/graph/model"
)

type ctxKey string

const loadersKey ctxKey = "dataloaders"

type Loaders struct {
	UserLoader     *UserBatchLoader
	LikeStatusLoader *LikeStatusBatchLoader
}

// UserBatchLoader collects user IDs across concurrent GraphQL field resolvers and fetches in a single SQL query
type UserBatchLoader struct {
	store *db.Store
	mu    sync.Mutex
	batch map[string][]chan *model.User
	timer *time.Timer
}

func NewUserBatchLoader(store *db.Store) *UserBatchLoader {
	return &UserBatchLoader{
		store: store,
		batch: make(map[string][]chan *model.User),
	}
}

func (l *UserBatchLoader) Load(ctx context.Context, userID string) (*model.User, error) {
	ch := make(chan *model.User, 1)

	l.mu.Lock()
	l.batch[userID] = append(l.batch[userID], ch)
	if len(l.batch) == 1 {
		// First key in batch, schedule flush
		l.timer = time.AfterFunc(2*time.Millisecond, func() {
			l.flush()
		})
	}
	l.mu.Unlock()

	select {
	case user := <-ch:
		return user, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (l *UserBatchLoader) flush() {
	l.mu.Lock()
	batch := l.batch
	l.batch = make(map[string][]chan *model.User)
	l.mu.Unlock()

	if len(batch) == 0 {
		return
	}

	uuids := make([]pgtype.UUID, 0, len(batch))
	for idStr := range batch {
		var u pgtype.UUID
		if err := u.Scan(idStr); err == nil {
			uuids = append(uuids, u)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	users, err := l.store.GetUsersByIDs(ctx, uuids)
	userMap := make(map[string]*model.User)
	if err == nil {
		for _, u := range users {
			idStr := sqlc.UUIDToString(u.ID)
			userMap[idStr] = &model.User{
				ID:          idStr,
				Email:       u.Email,
				Handle:      u.Handle,
				DisplayName: u.DisplayName,
				AvatarURL:   u.AvatarUrl,
				Bio:         u.Bio,
				CreatedAt:   u.CreatedAt.Time.Format(time.RFC3339),
			}
		}
	}

	for idStr, channels := range batch {
		usr := userMap[idStr]
		for _, ch := range channels {
			ch <- usr
		}
	}
}

// LikeStatusBatchLoader collects tweet IDs and checks whether the authenticated user liked each tweet
type LikeStatusBatchLoader struct {
	store  *db.Store
	userID string
	mu     sync.Mutex
	batch  map[string][]chan bool
	timer  *time.Timer
}

func NewLikeStatusBatchLoader(store *db.Store, userID string) *LikeStatusBatchLoader {
	return &LikeStatusBatchLoader{
		store:  store,
		userID: userID,
		batch:  make(map[string][]chan bool),
	}
}

func (l *LikeStatusBatchLoader) Load(ctx context.Context, tweetID string) (bool, error) {
	if l.userID == "" {
		return false, nil
	}

	ch := make(chan bool, 1)

	l.mu.Lock()
	l.batch[tweetID] = append(l.batch[tweetID], ch)
	if len(l.batch) == 1 {
		l.timer = time.AfterFunc(2*time.Millisecond, func() {
			l.flush()
		})
	}
	l.mu.Unlock()

	select {
	case liked := <-ch:
		return liked, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (l *LikeStatusBatchLoader) flush() {
	l.mu.Lock()
	batch := l.batch
	l.batch = make(map[string][]chan bool)
	l.mu.Unlock()

	if len(batch) == 0 {
		return
	}

	var userUUID pgtype.UUID
	if err := userUUID.Scan(l.userID); err != nil {
		for _, channels := range batch {
			for _, ch := range channels {
				ch <- false
			}
		}
		return
	}

	tweetUUIDs := make([]pgtype.UUID, 0, len(batch))
	for tID := range batch {
		var u pgtype.UUID
		if err := u.Scan(tID); err == nil {
			tweetUUIDs = append(tweetUUIDs, u)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	likedIDs, err := l.store.GetUserLikedTweetIDs(ctx, sqlc.GetUserLikedTweetIDsParams{
		UserID:  userUUID,
		Dollar2: tweetUUIDs,
	})

	likedSet := make(map[string]bool)
	if err == nil {
		for _, id := range likedIDs {
			likedSet[sqlc.UUIDToString(id)] = true
		}
	}

	for tweetID, channels := range batch {
		isLiked := likedSet[tweetID]
		for _, ch := range channels {
			ch <- isLiked
		}
	}
}

// Middleware attaches fresh per-request DataLoader instances to the context
func Middleware(store *db.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID := ""
			if currentUser := auth.GetUser(r.Context()); currentUser != nil {
				userID = sqlc.UUIDToString(currentUser.ID)
			}

			loaders := &Loaders{
				UserLoader:       NewUserBatchLoader(store),
				LikeStatusLoader: NewLikeStatusBatchLoader(store, userID),
			}

			ctx := context.WithValue(r.Context(), loadersKey, loaders)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// For returns DataLoader from request context
func For(ctx context.Context) *Loaders {
	loaders, ok := ctx.Value(loadersKey).(*Loaders)
	if !ok {
		return nil
	}
	return loaders
}
