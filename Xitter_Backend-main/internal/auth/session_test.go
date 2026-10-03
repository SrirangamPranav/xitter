package auth_test

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/monis/twitter-backend-go/internal/auth"
)

func TestGenerateSecureToken(t *testing.T) {
	tok1, err := auth.GenerateSecureToken()
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	tok2, err := auth.GenerateSecureToken()
	if err != nil {
		t.Fatalf("unexpected error generating token: %v", err)
	}

	if tok1 == tok2 {
		t.Fatalf("expected tokens to be unique, got identical: %s", tok1)
	}

	// Token is 32 bytes hex encoded -> 64 hex characters
	if len(tok1) != 64 {
		t.Errorf("expected token length 64, got %d", len(tok1))
	}

	decoded, err := hex.DecodeString(tok1)
	if err != nil {
		t.Errorf("expected valid hex string, failed decode: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("expected 32 decoded bytes, got %d", len(decoded))
	}
}

func TestSessionExpirationLogic(t *testing.T) {
	now := time.Now().UTC()
	ttl := 7 * 24 * time.Hour
	expiresAt := now.Add(ttl)

	session := &auth.Session{
		ID:        "test-token",
		UserID:    "user-123",
		Email:     "test@example.com",
		UserAgent: "Mozilla/5.0",
		IPAddress: "127.0.0.1",
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}

	if session.ExpiresAt.Before(now) {
		t.Errorf("session should not be expired upon creation")
	}

	if session.UserID != "user-123" {
		t.Errorf("expected user-123, got %s", session.UserID)
	}
}
