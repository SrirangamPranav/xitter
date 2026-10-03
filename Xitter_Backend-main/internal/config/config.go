package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port              string
	Env               string
	AppURL            string
	DatabaseURL       string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration

	// Redis 1: Dedicated Session Storage
	RedisSessionURL string
	SessionTTL      time.Duration
	CookieName      string
	CookieSecret    string
	CookieSecure    bool
	CookieSameSite  string

	// Redis 2: Dedicated LRU Query Cache
	RedisCacheURL   string
	CacheDefaultTTL time.Duration
	FeedCacheTTL    time.Duration
	UserCacheTTL    time.Duration

	// Google OAuth
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
	EnableDevAuth      bool
}

func Load() *Config {
	// Attempt loading .env, ignore if missing (e.g. in container)
	if err := godotenv.Load(); err != nil {
		log.Println("[Config] .env file not found, reading from system environment")
	}

	return &Config{
		Port:              getEnv("PORT", "8080"),
		Env:               getEnv("ENV", "development"),
		AppURL:            getEnv("APP_URL", "http://localhost:8080"),
		DatabaseURL:       getEnv("DATABASE_URL", "postgres://postgres:postgrespassword@localhost:5432/twitter_db?sslmode=disable"),
		DBMaxOpenConns:    getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:    getEnvInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetime: getEnvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),

		RedisSessionURL: getEnv("REDIS_SESSION_URL", "redis://localhost:6379/0"),
		SessionTTL:      time.Duration(getEnvInt("SESSION_TTL_HOURS", 168)) * time.Hour,
		CookieName:      getEnv("COOKIE_NAME", "twitter_session"),
		CookieSecret:    getEnv("COOKIE_SECRET", "super-secret-twitter-session-key-32b"),
		CookieSecure:    getEnvBool("COOKIE_SECURE", false),
		CookieSameSite:  getEnv("COOKIE_SAME_SITE", "lax"),

		RedisCacheURL:   getEnv("REDIS_CACHE_URL", "redis://localhost:6380/0"),
		CacheDefaultTTL: time.Duration(getEnvInt("CACHE_DEFAULT_TTL_SECONDS", 300)) * time.Second,
		FeedCacheTTL:    time.Duration(getEnvInt("FEED_CACHE_TTL_SECONDS", 60)) * time.Second,
		UserCacheTTL:    time.Duration(getEnvInt("USER_CACHE_TTL_SECONDS", 900)) * time.Second,

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/auth/google/callback"),
		EnableDevAuth:      getEnvBool("ENABLE_DEV_AUTH", true),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		b, err := strconv.ParseBool(val)
		if err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		d, err := time.ParseDuration(val)
		if err == nil {
			return d
		}
	}
	return fallback
}
