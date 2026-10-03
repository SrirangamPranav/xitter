package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/monis/twitter-backend-go/internal/auth"
	"github.com/monis/twitter-backend-go/internal/cache"
	"github.com/monis/twitter-backend-go/internal/config"
	"github.com/monis/twitter-backend-go/internal/db"
	"github.com/monis/twitter-backend-go/internal/graph"
	"github.com/monis/twitter-backend-go/internal/graph/dataloader"
	"github.com/monis/twitter-backend-go/internal/pubsub"
	"github.com/redis/go-redis/v9"
)

func main() {
	log.Println("=========================================================")
	log.Println(" Starting Twitter-like Platform Backend (Go + GraphQL) ")
	log.Println("=========================================================")

	// 1. Load Configuration
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 2. Initialize PostgreSQL Database Pool
	log.Printf("[Database] Connecting to PostgreSQL at: %s\n", cfg.DatabaseURL)
	store, err := db.NewPool(ctx, cfg)
	if err != nil {
		log.Printf("[Database] Warning: PostgreSQL not reachable yet (%v). Ensure Postgres is running.\n", err)
	} else {
		defer store.Close()

		// Attempt auto-migration if migration directory exists
		migrationDirs := []string{
			"internal/db/migrations",
			"/app/internal/db/migrations",
			"../internal/db/migrations",
		}
		for _, dir := range migrationDirs {
			if _, statErr := os.Stat(dir); statErr == nil {
				if migErr := store.RunMigrations(ctx, dir); migErr != nil {
					log.Printf("[Database] Auto-migration warning: %v\n", migErr)
				}
				break
			}
		}
	}

	// 3. Connect to Dedicated Redis Instance 1: Session Management
	log.Printf("[Redis] Connecting to Session Redis (Non-eviction): %s\n", cfg.RedisSessionURL)
	sessionOpt, err := redis.ParseURL(cfg.RedisSessionURL)
	if err != nil {
		log.Fatalf("Invalid Redis Session URL: %v", err)
	}
	sessionRedis := redis.NewClient(sessionOpt)
	defer sessionRedis.Close()

	if pingErr := sessionRedis.Ping(ctx).Err(); pingErr != nil {
		log.Printf("[Redis] Warning: Session Redis not reachable yet (%v)\n", pingErr)
	} else {
		log.Println("[Redis] Session Redis connected successfully")
	}
	sessionMgr := auth.NewSessionManager(sessionRedis, cfg)

	// 4. Connect to Dedicated Redis Instance 2: LRU Query Cache
	log.Printf("[Redis] Connecting to Cache Redis (allkeys-lru): %s\n", cfg.RedisCacheURL)
	cacheOpt, err := redis.ParseURL(cfg.RedisCacheURL)
	if err != nil {
		log.Fatalf("Invalid Redis Cache URL: %v", err)
	}
	cacheRedis := redis.NewClient(cacheOpt)
	defer cacheRedis.Close()

	if pingErr := cacheRedis.Ping(ctx).Err(); pingErr != nil {
		log.Printf("[Redis] Warning: Cache Redis not reachable yet (%v)\n", pingErr)
	} else {
		log.Println("[Redis] Cache Redis connected successfully")
	}
	queryCache := cache.NewQueryCache(cacheRedis, cfg)

	// 5. Connect Redis Pub/Sub for WebSockets
	pubsubMgr := pubsub.NewPubSubManager(cacheRedis)

	// 6. Setup Auth Services & Middleware
	oauthSvc := auth.NewOAuthService(cfg, sessionMgr, store)
	authMiddleware := auth.NewMiddleware(sessionMgr, store, cfg)

	// 7. Initialize GraphQL Resolvers & Handler
	resolver := graph.NewResolver(store, sessionMgr, queryCache, pubsubMgr, cfg)
	gqlHandler := graph.NewGraphQLHandler(resolver)

	// 8. Build HTTP Router (Chi)
	r := chi.NewRouter()

	// Global Middlewares
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(chiMiddleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.Timeout(60 * time.Second))

	// CORS Configuration (Allow credentials for HttpOnly cookies)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:*", "http://127.0.0.1:*", "https://studio.apollographql.com"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Auth and DataLoader Context Injection
	r.Use(authMiddleware.SessionMiddleware)
	if store != nil {
		r.Use(dataloader.Middleware(store))
	}

	// Health check endpoint (reports PostgreSQL, Dual Redis status, and LRU Cache metrics)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		hCtx, hCancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer hCancel()

		dbStatus := "healthy"
		if store == nil || store.Pool.Ping(hCtx) != nil {
			dbStatus = "unhealthy"
		}

		sessionRedisStatus := "healthy"
		if sessionRedis.Ping(hCtx).Err() != nil {
			sessionRedisStatus = "unhealthy"
		}

		cacheRedisStatus := "healthy"
		if cacheRedis.Ping(hCtx).Err() != nil {
			cacheRedisStatus = "unhealthy"
		}

		cacheStats := queryCache.GetStats()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "online",
			"time":   time.Now().UTC().Format(time.RFC3339),
			"components": map[string]string{
				"postgres":      dbStatus,
				"redis_session": sessionRedisStatus,
				"redis_cache":   cacheRedisStatus,
			},
			"lru_cache_stats": cacheStats,
		})
	})

	// OAuth Endpoints
	r.Get("/auth/google/login", oauthSvc.HandleGoogleLogin)
	r.Get("/auth/google/callback", oauthSvc.HandleGoogleCallback)
	r.Get("/auth/dev/login", oauthSvc.HandleDevLogin)

	// GraphQL Playground UI
	r.Get("/", graph.NewPlaygroundHandler("Twitter GraphQL Playground", "/query"))

	// GraphQL API (Queries, Mutations, and WebSocket Subscriptions)
	r.Handle("/query", gqlHandler)

	// 9. Start Server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf(" GraphQL Playground running at http://localhost:%s/\n", cfg.Port)
		log.Printf(" GraphQL API Endpoint: http://localhost:%s/query\n", cfg.Port)
		log.Printf(" Dev Authentication: http://localhost:%s/auth/dev/login\n", cfg.Port)
		log.Printf(" Health & Cache Stats: http://localhost:%s/health\n", cfg.Port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// 10. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("[Server] Shutting down gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Server] Forced shutdown: %v\n", err)
	}
	log.Println("[Server] Server exited successfully.")
}

func init() {
	// Silence unused import warnings if needed
	_ = filepath.Join
}
