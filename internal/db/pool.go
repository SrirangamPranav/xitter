package db

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/monis/twitter-backend-go/internal/config"
	"github.com/monis/twitter-backend-go/internal/db/sqlc"
)

type Store struct {
	*sqlc.Queries
	Pool *pgxpool.Pool
}

// NewPool initializes a high-performance pgxpool connection pool
func NewPool(ctx context.Context, cfg *config.Config) (*Store, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	poolConfig.MaxConns = int32(cfg.DBMaxOpenConns)
	poolConfig.MinConns = int32(cfg.DBMaxIdleConns)
	poolConfig.MaxConnLifetime = cfg.DBConnMaxLifetime
	poolConfig.MaxConnIdleTime = 15 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connection
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("[Database] Successfully connected to PostgreSQL connection pool")

	return &Store{
		Queries: sqlc.New(pool),
		Pool:    pool,
	}, nil
}

// RunMigrations executes all SQL migration files found in migrations directory
func (s *Store) RunMigrations(ctx context.Context, migrationsDir string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("could not read migrations directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".up.sql") {
			filePath := filepath.Join(migrationsDir, entry.Name())
			content, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed reading migration %s: %w", entry.Name(), err)
			}

			log.Printf("[Database] Applying migration: %s\n", entry.Name())
			_, err = s.Pool.Exec(ctx, string(content))
			if err != nil {
				return fmt.Errorf("error executing migration %s: %w", entry.Name(), err)
			}
		}
	}
	log.Println("[Database] All migrations applied successfully")
	return nil
}

func (s *Store) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}
