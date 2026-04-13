package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/whozpj/argus/server/internal/store"
)

// OAuthConfig holds OAuth app credentials and the server's public base URL.
type OAuthConfig struct {
	BaseURL            string // e.g. "https://argus.app" or "http://localhost:4000"
	UIURL              string // e.g. "http://localhost:3000" — browser is sent here after web OAuth
	GitHubClientID     string
	GitHubClientSecret string
	GoogleClientID     string
	GoogleClientSecret string
}

type authHandlers struct {
	db     *store.DB
	cfg    OAuthConfig
	github *GitHubOAuth
	google *GoogleOAuth
}

// RegisterRoutes wires all auth routes onto mux. No auth middleware on these routes.
func RegisterRoutes(mux *http.ServeMux, db *store.DB, cfg OAuthConfig) {
	h := &authHandlers{
		db:     db,
		cfg:    cfg,
		github: NewGitHubOAuth(cfg.GitHubClientID, cfg.GitHubClientSecret),
		google: NewGoogleOAuth(cfg.GoogleClientID, cfg.GoogleClientSecret),
	}
	mux.HandleFunc("GET /auth/github", h.githubLogin)
	mux.HandleFunc("GET /auth/github/callback", h.githubCallback)
	mux.HandleFunc("GET /auth/google", h.googleLogin)
	mux.HandleFunc("GET /auth/google/callback", h.googleCallback)
	mux.HandleFunc("GET /auth/cli", h.cliLogin)
	mux.HandleFunc("POST /api/v1/auth/token", h.tokenExchange)
}

func (h *authHandlers) githubLogin(w http.ResponseWriter, r *http.Request) {
	state := randomHex(16)
	setStateCookie(w, state)
	redirectURI := h.cfg.BaseURL + "/auth/github/callback"
	http.Redirect(w, r, h.github.AuthURL(state, redirectURI), http.StatusFound)
}

func (h *authHandlers) githubCallback(w http.ResponseWriter, r *http.Request) {
	if !validateState(r) {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	tok, err := h.github.ExchangeCode(r.Context(), code)
	if err != nil {
		slog.Error("github exchange code", "err", err)
		http.Error(w, "oauth error", http.StatusInternalServerError)
		return
	}
	ghUser, err := h.github.GetUser(r.Context(), tok)
	if err != nil {
		slog.Error("github get user", "err", err)
		http.Error(w, "oauth error", http.StatusInternalServerError)
		return
	}
	email := ghUser.Email
	if email == "" {
		email = fmt.Sprintf("%d@github.com", ghUser.ID)
	}
	userID, err := h.db.UpsertUser(email, fmt.Sprintf("%d", ghUser.ID), "")
	if err != nil {
		slog.Error("upsert user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.issueJWTAndRedirect(w, r, userID)
}

func (h *authHandlers) googleLogin(w http.ResponseWriter, r *http.Request) {
	state := randomHex(16)
	setStateCookie(w, state)
	redirectURI := h.cfg.BaseURL + "/auth/google/callback"
	http.Redirect(w, r, h.google.LoginURL(state, redirectURI), http.StatusFound)
}

func (h *authHandlers) googleCallback(w http.ResponseWriter, r *http.Request) {
	if !validateState(r) {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	redirectURI := h.cfg.BaseURL + "/auth/google/callback"
	tok, err := h.google.ExchangeCode(r.Context(), code, redirectURI)
	if err != nil {
		slog.Error("google exchange code", "err", err)
		http.Error(w, "oauth error", http.StatusInternalServerError)
		return
	}
	gUser, err := h.google.GetUser(r.Context(), tok)
	if err != nil {
		slog.Error("google get user", "err", err)
		http.Error(w, "oauth error", http.StatusInternalServerError)
		return
	}
	userID, err := h.db.UpsertUser(gUser.Email, "", gUser.Sub)
	if err != nil {
		slog.Error("upsert user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.issueJWTAndRedirect(w, r, userID)
}

// cliLogin handles GET /auth/cli?redirect=http://localhost:<port>/callback.
// If the user already has a valid JWT cookie, creates a short-lived code immediately.
// Otherwise, saves the redirect destination in a cookie and starts GitHub OAuth.
func (h *authHandlers) cliLogin(w http.ResponseWriter, r *http.Request) {
	redirect := r.URL.Query().Get("redirect")
	if redirect == "" {
		http.Error(w, "redirect parameter required", http.StatusBadRequest)
		return
	}

	// Already logged in — issue code directly.
	if c, err := r.Cookie("argus_token"); err == nil {
		if userID, err := ValidateToken(c.Value); err == nil {
			h.createSessionCodeAndRedirect(w, r, userID, redirect)
			return
		}
	}

	// Not logged in — save CLI redirect, start GitHub OAuth.
	http.SetCookie(w, &http.Cookie{
		Name:     "argus_cli_redirect",
		Value:    redirect,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
	})
	state := randomHex(16)
	setStateCookie(w, state)
	redirectURI := h.cfg.BaseURL + "/auth/github/callback"
	http.Redirect(w, r, h.github.AuthURL(state, redirectURI), http.StatusFound)
}

// tokenExchange handles POST /api/v1/auth/token.
// The CLI sends {"code": "<short-lived code>"} and receives {"token": "<JWT>", "email": "..."}.
func (h *authHandlers) tokenExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		http.Error(w, "code required", http.StatusBadRequest)
		return
	}
	userID, ok, err := h.db.ConsumeOAuthSession(req.Code)
	if err != nil {
		slog.Error("consume oauth session", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "invalid or expired code", http.StatusUnauthorized)
		return
	}
	user, err := h.db.GetUserByID(userID)
	if err != nil {
		slog.Error("get user", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	jwtTok, err := IssueToken(userID)
	if err != nil {
		slog.Error("issue token", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"token": jwtTok,
		"email": user.Email,
	})
}

// issueJWTAndRedirect handles the post-OAuth redirect for both CLI and web flows.
//
// CLI flow: if argus_cli_redirect cookie is present, creates a short-lived code
// and sends the browser to the CLI's local callback server.
//
// Web flow: keeps the argus_token cookie (so /auth/cli can reuse an existing session),
// then creates a short-lived code and sends the browser to the UI's /auth/callback page.
func (h *authHandlers) issueJWTAndRedirect(w http.ResponseWriter, r *http.Request, userID string) {
	tok, err := IssueToken(userID)
	if err != nil {
		slog.Error("issue token", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Keep the server-side cookie so /auth/cli can skip re-auth on repeat CLI logins.
	http.SetCookie(w, &http.Cookie{
		Name:     "argus_token",
		Value:    tok,
		Path:     "/",
		MaxAge:   30 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// CLI flow — redirect back to the CLI's local callback server.
	if c, err := r.Cookie("argus_cli_redirect"); err == nil {
		http.SetCookie(w, &http.Cookie{Name: "argus_cli_redirect", MaxAge: -1, Path: "/"})
		h.createSessionCodeAndRedirect(w, r, userID, c.Value)
		return
	}

	// Web flow — redirect the browser to the UI with a one-time code.
	uiURL := h.cfg.UIURL
	if uiURL == "" {
		uiURL = "http://localhost:3000"
	}
	h.createSessionCodeAndRedirect(w, r, userID, uiURL+"/auth/callback")
}

// createSessionCodeAndRedirect stores a short-lived code in oauth_sessions and
// redirects the browser to `redirectTo?code=<code>`.
func (h *authHandlers) createSessionCodeAndRedirect(w http.ResponseWriter, r *http.Request, userID, redirectTo string) {
	code := randomHex(32)
	if err := h.db.CreateOAuthSession(code, userID, time.Now().Add(10*time.Minute)); err != nil {
		slog.Error("create oauth session", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, redirectTo+"?code="+code, http.StatusFound)
}

// setStateCookie sets the short-lived argus_oauth_state cookie used for CSRF protection.
func setStateCookie(w http.ResponseWriter, state string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "argus_oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
	})
}

// validateState checks that the state query param matches the argus_oauth_state cookie.
func validateState(r *http.Request) bool {
	state := r.URL.Query().Get("state")
	c, err := r.Cookie("argus_oauth_state")
	return err == nil && c.Value == state && state != ""
}

// randomHex generates n random bytes as a hex string.
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b) //nolint:errcheck
	return hex.EncodeToString(b)
}
