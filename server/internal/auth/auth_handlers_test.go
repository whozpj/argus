package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/whozpj/argus/server/internal/auth"
)

func TestAuthHandlers_GitHubLoginRedirects(t *testing.T) {
	db := newTestDB(t)
	mux := http.NewServeMux()
	auth.RegisterRoutes(mux, db, auth.OAuthConfig{
		BaseURL:            "http://localhost:4000",
		GitHubClientID:     "gh-test-id",
		GitHubClientSecret: "gh-secret",
	})

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/github", nil))

	if rr.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "github.com/login/oauth/authorize") {
		t.Errorf("Location should be GitHub: %s", loc)
	}
	if !strings.Contains(loc, "gh-test-id") {
		t.Errorf("Location should contain client_id: %s", loc)
	}
	// State cookie must be set
	found := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == "argus_oauth_state" {
			found = true
		}
	}
	if !found {
		t.Error("argus_oauth_state cookie not set")
	}
}

func TestAuthHandlers_GoogleLoginRedirects(t *testing.T) {
	db := newTestDB(t)
	mux := http.NewServeMux()
	auth.RegisterRoutes(mux, db, auth.OAuthConfig{
		BaseURL:            "http://localhost:4000",
		GoogleClientID:     "goog-test-id",
		GoogleClientSecret: "goog-secret",
	})

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/google", nil))

	if rr.Code != http.StatusFound {
		t.Errorf("status = %d, want 302", rr.Code)
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "accounts.google.com") {
		t.Errorf("Location should be Google: %s", loc)
	}
}

func TestAuthHandlers_TokenExchange_Success(t *testing.T) {
	t.Setenv("JWT_SECRET", "handler-test-secret")
	db := newTestDB(t)
	mux := http.NewServeMux()
	auth.RegisterRoutes(mux, db, auth.OAuthConfig{BaseURL: "http://localhost:4000"})

	// Create user and oauth session in DB
	userID, err := db.UpsertUser("cli@example.com", "gh-cli-1", "")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}
	code := "test-cli-code-12345"
	if err := db.CreateOAuthSession(code, userID, time.Now().Add(10*time.Minute)); err != nil {
		t.Fatalf("create oauth session: %v", err)
	}

	body := `{"code":"` + code + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["token"] == "" {
		t.Error("token should not be empty")
	}
	if resp["email"] != "cli@example.com" {
		t.Errorf("email = %q, want cli@example.com", resp["email"])
	}

	// Verify the returned token is a valid JWT
	uid, err := auth.ValidateToken(resp["token"])
	if err != nil {
		t.Fatalf("returned token invalid: %v", err)
	}
	if uid != userID {
		t.Errorf("token uid = %q, want %q", uid, userID)
	}
}

func TestAuthHandlers_TokenExchange_InvalidCode(t *testing.T) {
	db := newTestDB(t)
	mux := http.NewServeMux()
	auth.RegisterRoutes(mux, db, auth.OAuthConfig{BaseURL: "http://localhost:4000"})

	body := `{"code":"nonexistent-code"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token",
		strings.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestAuthHandlers_TokenExchange_CodeConsumedOnce(t *testing.T) {
	t.Setenv("JWT_SECRET", "handler-test-secret2")
	db := newTestDB(t)
	mux := http.NewServeMux()
	auth.RegisterRoutes(mux, db, auth.OAuthConfig{BaseURL: "http://localhost:4000"})

	userID, _ := db.UpsertUser("once@example.com", "gh-once", "")
	code := "one-time-code-xyz"
	db.CreateOAuthSession(code, userID, time.Now().Add(10*time.Minute)) //nolint:errcheck

	// First use — success
	body := `{"code":"` + code + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", strings.NewReader(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first use: status = %d, want 200", rr.Code)
	}

	// Second use — must fail (code consumed)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/token", strings.NewReader(body))
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("second use: status = %d, want 401", rr2.Code)
	}
}
