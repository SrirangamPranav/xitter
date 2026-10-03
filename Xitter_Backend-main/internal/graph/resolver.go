package graph

import (
	"github.com/monis/twitter-backend-go/internal/auth"
	"github.com/monis/twitter-backend-go/internal/cache"
	"github.com/monis/twitter-backend-go/internal/config"
	"github.com/monis/twitter-backend-go/internal/db"
	"github.com/monis/twitter-backend-go/internal/pubsub"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	Store      *db.Store
	SessionMgr *auth.SessionManager
	Cache      *cache.QueryCache
	PubSub     *pubsub.PubSubManager
	Config     *config.Config
}

func NewResolver(
	store *db.Store,
	sessionMgr *auth.SessionManager,
	cache *cache.QueryCache,
	pubsub *pubsub.PubSubManager,
	cfg *config.Config,
) *Resolver {
	return &Resolver{
		Store:      store,
		SessionMgr: sessionMgr,
		Cache:      cache,
		PubSub:     pubsub,
		Config:     cfg,
	}
}
