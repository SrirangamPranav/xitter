package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/monis/twitter-backend-go/internal/config"
	"github.com/monis/twitter-backend-go/internal/db"
	"github.com/monis/twitter-backend-go/internal/db/sqlc"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type OAuthService struct {
	oauthConfig *oauth2.Config
	sessionMgr  *SessionManager
	store       *db.Store
	cfg         *config.Config
}

type GoogleUserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

func NewOAuthService(cfg *config.Config, sessionMgr *SessionManager, store *db.Store) *OAuthService {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.profile",
			"https://www.googleapis.com/auth/userinfo.email",
		},
		Endpoint: google.Endpoint,
	}

	return &OAuthService{
		oauthConfig: oauthConfig,
		sessionMgr:  sessionMgr,
		store:       store,
		cfg:         cfg,
	}
}

// HandleGoogleLogin initiates the Google OAuth 2.0 flow
func (s *OAuthService) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		http.Error(w, "Failed to generate oauth state", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(stateBytes)

	// Save state in temporary cookie for CSRF validation
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300, // 5 minutes
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	url := s.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// HandleGoogleCallback processes Google's authorization response
func (s *OAuthService) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify CSRF state
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "Invalid OAuth state token", http.StatusUnauthorized)
		return
	}

	// Delete state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Code not found in callback", http.StatusBadRequest)
		return
	}

	token, err := s.oauthConfig.Exchange(ctx, code)
	if err != nil {
		log.Printf("[OAuth] Code exchange failed: %v", err)
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}

	client := s.oauthConfig.Client(ctx, token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		log.Printf("[OAuth] Failed fetching user info: %v", err)
		http.Error(w, "Failed to retrieve user profile", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	var gUser GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&gUser); err != nil {
		http.Error(w, "Failed to parse Google user profile", http.StatusInternalServerError)
		return
	}

	// Generate a unique handle from email prefix if new
	handlePrefix := strings.Split(gUser.Email, "@")[0]
	handle := fmt.Sprintf("%s_%d", handlePrefix, time.Now().Unix()%1000)

	// Upsert user into database via sqlc
	user, err := s.store.UpsertGoogleUser(ctx, sqlc.UpsertGoogleUserParams{
		GoogleID:    pgtype.Text{String: gUser.ID, Valid: true},
		Email:       gUser.Email,
		Handle:      handle,
		DisplayName: gUser.Name,
		AvatarUrl:   gUser.Picture,
	})
	if err != nil {
		log.Printf("[OAuth] Database upsert failed: %v", err)
		http.Error(w, "Failed to persist user profile", http.StatusInternalServerError)
		return
	}

	// Create Redis session
	userIDStr := sqlc.UUIDToString(user.ID)
	_, sessionToken, err := s.sessionMgr.CreateSession(
		ctx,
		userIDStr,
		user.Email,
		r.UserAgent(),
		r.RemoteAddr,
	)
	if err != nil {
		log.Printf("[OAuth] Session creation failed: %v", err)
		http.Error(w, "Failed to generate session", http.StatusInternalServerError)
		return
	}

	// Set HttpOnly session cookie
	s.sessionMgr.SetSessionCookie(w, sessionToken)

	// Redirect to web app home / playground
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// HandleDevLogin provides a local development login endpoint without needing active Google credentials
func (s *OAuthService) HandleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.EnableDevAuth {
		http.Error(w, "Dev authentication is disabled", http.StatusForbidden)
		return
	}

	ctx := r.Context()
	email := r.URL.Query().Get("email")
	if email == "" {
		email = "developer@example.com"
	}
	handle := r.URL.Query().Get("handle")
	if handle == "" {
		handle = strings.Split(email, "@")[0]
	}
	name := r.URL.Query().Get("name")
	if name == "" {
		name = strings.Title(handle)
	}

	user, err := s.store.GetUserByEmail(ctx, email)
	if err != nil {
		// Create if not exists
		user, err = s.store.CreateUser(ctx, sqlc.CreateUserParams{
			Email:       email,
			Handle:      handle,
			DisplayName: name,
			AvatarUrl:   fmt.Sprintf("https://api.dicebear.com/7.x/identicon/svg?seed=%s", handle),
			Bio:         "Twitter Backend Developer",
		})
		if err != nil {
			log.Printf("[DevAuth] Failed to create dev user: %v", err)
			http.Error(w, fmt.Sprintf("Error creating dev user: %v", err), http.StatusInternalServerError)
			return
		}
	}

	userIDStr := sqlc.UUIDToString(user.ID)
	_, sessionToken, err := s.sessionMgr.CreateSession(
		ctx,
		userIDStr,
		user.Email,
		r.UserAgent(),
		r.RemoteAddr,
	)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	s.sessionMgr.SetSessionCookie(w, sessionToken)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":        "authenticated",
		"user":          user,
		"session_token": sessionToken,
		"message":       "Session cookie set. You are now logged in!",
	})
}
