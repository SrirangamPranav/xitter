package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/monis/twitter-backend-go/internal/config"
	"github.com/redis/go-redis/v9"
)

var (
	ErrSessionNotFound = errors.New("session not found or expired")
)

type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SessionManager struct {
	client *redis.Client
	cfg    *config.Config
}

func NewSessionManager(client *redis.Client, cfg *config.Config) *SessionManager {
	return &SessionManager{
		client: client,
		cfg:    cfg,
	}
}

func sessionKey(token string) string {
	return fmt.Sprintf("session:%s", token)
}

func userSessionsKey(userID string) string {
	return fmt.Sprintf("user_sessions:%s", userID)
}

// GenerateSecureToken creates a cryptographically secure 32-byte hex token (64 chars)
func GenerateSecureToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateSession initializes a new session on the dedicated session Redis instance
func (sm *SessionManager) CreateSession(ctx context.Context, userID, email, userAgent, ip string) (*Session, string, error) {
	token, err := GenerateSecureToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate session token: %w", err)
	}

	now := time.Now().UTC()
	session := &Session{
		ID:        token,
		UserID:    userID,
		Email:     email,
		UserAgent: userAgent,
		IPAddress: ip,
		CreatedAt: now,
		ExpiresAt: now.Add(sm.cfg.SessionTTL),
	}

	data, err := json.Marshal(session)
	if err != nil {
		return nil, "", fmt.Errorf("failed to marshal session data: %w", err)
	}

	pipe := sm.client.TxPipeline()
	// Set the session payload with TTL-based expiration
	pipe.Set(ctx, sessionKey(token), data, sm.cfg.SessionTTL)
	// Track session token in user's active session set
	pipe.SAdd(ctx, userSessionsKey(userID), token)
	// Give user set a TTL matching session duration (extended when new sessions added)
	pipe.Expire(ctx, userSessionsKey(userID), sm.cfg.SessionTTL)

	if _, err := pipe.Exec(ctx); err != nil {
		return nil, "", fmt.Errorf("failed to save session in redis: %w", err)
	}

	return session, token, nil
}

// GetSession retrieves the active session from Redis
func (sm *SessionManager) GetSession(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, ErrSessionNotFound
	}

	val, err := sm.client.Get(ctx, sessionKey(token)).Result()
	if err == redis.Nil {
		return nil, ErrSessionNotFound
	} else if err != nil {
		return nil, fmt.Errorf("redis error fetching session: %w", err)
	}

	var session Session
	if err := json.Unmarshal([]byte(val), &session); err != nil {
		return nil, fmt.Errorf("failed to decode session payload: %w", err)
	}

	return &session, nil
}

// TouchSession extends session TTL upon active request
func (sm *SessionManager) TouchSession(ctx context.Context, token string) error {
	return sm.client.Expire(ctx, sessionKey(token), sm.cfg.SessionTTL).Err()
}

// RevokeSession invalidates a single session on logout or revocation
func (sm *SessionManager) RevokeSession(ctx context.Context, token string) error {
	session, err := sm.GetSession(ctx, token)
	if err != nil && !errors.Is(err, ErrSessionNotFound) {
		return err
	}

	pipe := sm.client.TxPipeline()
	pipe.Del(ctx, sessionKey(token))
	if session != nil {
		pipe.SRem(ctx, userSessionsKey(session.UserID), token)
	}

	_, err = pipe.Exec(ctx)
	return err
}

// RevokeAllUserSessions enables server-side revocation across all devices for a user
func (sm *SessionManager) RevokeAllUserSessions(ctx context.Context, userID string) error {
	tokens, err := sm.client.SMembers(ctx, userSessionsKey(userID)).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to fetch user session tokens: %w", err)
	}

	if len(tokens) == 0 {
		return nil
	}

	pipe := sm.client.TxPipeline()
	for _, tok := range tokens {
		pipe.Del(ctx, sessionKey(tok))
	}
	pipe.Del(ctx, userSessionsKey(userID))

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to revoke all sessions for user %s: %w", userID, err)
	}

	return nil
}

// SetSessionCookie sets a secure, HttpOnly session cookie
func (sm *SessionManager) SetSessionCookie(w http.ResponseWriter, token string) {
	sameSite := http.SameSiteLaxMode
	switch strings.ToLower(sm.cfg.CookieSameSite) {
	case "strict":
		sameSite = http.SameSiteStrictMode
	case "none":
		sameSite = http.SameSiteNoneMode
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sm.cfg.CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sm.cfg.SessionTTL.Seconds()),
		HttpOnly: true,
		Secure:   sm.cfg.CookieSecure,
		SameSite: sameSite,
	})
}

// ClearSessionCookie clears the session cookie from the client
func (sm *SessionManager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sm.cfg.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   sm.cfg.CookieSecure,
	})
}
