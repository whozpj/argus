# Plan 2: Auth & API Keys

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add JWT authentication, GitHub + Google OAuth, API key generation/validation, and project-management API endpoints to the `cloud` branch server.

**Architecture:** A new `server/internal/auth/` package provides JWT issue/validate, OAuth helpers (GitHub + Google), request middleware (RequireJWT, ResolveProject), and HTTP handlers for all auth routes. The existing ingest and baselines handlers are updated to read projectID from request context (set by middleware) instead of the hardcoded `"self-hosted"` sentinel, which is preserved as the no-auth fallback for backward compatibility.

**Tech Stack:** Go 1.26, `github.com/golang-jwt/jwt/v5` (HS256 tokens), SHA-256 for API key hashing (stdlib `crypto/sha256`), `crypto/rand` for key generation, `testcontainers-go` for integration tests, standard `net/http` ServeMux with Go 1.22 method+path patterns.

---

## File Structure

**New files:**
- `server/internal/auth/jwt.go` — `IssueToken`, `ValidateToken`
- `server/internal/auth/middleware.go` — context keys, `RequireJWT`, `ResolveProject`, `GenerateAPIKey`, `HashAPIKey`
- `server/internal/auth/oauth_github.go` — `GitHubOAuth` struct, `AuthURL`, `ExchangeCode`, `GetUser`
- `server/internal/auth/oauth_google.go` — `GoogleOAuth` struct, `LoginURL`, `ExchangeCode`, `GetUser`
- `server/internal/auth/auth_handlers.go` — `OAuthConfig`, `RegisterRoutes`, OAuth redirect/callback handlers, `/auth/cli`, `POST /api/v1/auth/token`
- `server/internal/auth/project_handlers.go` — `RegisterProjectRoutes`, `/api/v1/me`, `/api/v1/projects`, `/api/v1/projects/{id}/keys`
- `server/internal/auth/testhelper_test.go` — `newTestDB(t)` (testcontainers, same pattern as other packages)
- `server/internal/auth/jwt_test.go`
- `server/internal/auth/middleware_test.go`
- `server/internal/auth/oauth_github_test.go`
- `server/internal/auth/oauth_google_test.go`
- `server/internal/auth/auth_handlers_test.go`
- `server/internal/auth/project_handlers_test.go`
- `server/internal/ingest/testhelper_test.go`
- `server/internal/ingest/handler_test.go`

**Modified files:**
- `server/internal/ingest/handler.go` — read projectID from context via `auth.ProjectIDFromContext`
- `server/internal/api/baselines.go` — read projectID from context via `auth.ProjectIDFromContext`
- `server/cmd/main.go` — wire new routes and middleware

---

## Design notes

**API key hashing:** SHA-256 is used instead of bcrypt. The design spec mentioned bcrypt, but bcrypt at cost 10 takes ~100 ms — unacceptable for an ingest endpoint hit on every LLM call. API keys are 32 random bytes (256-bit entropy) so SHA-256 is cryptographically sufficient; bcrypt's brute-force resistance is only needed for low-entropy user passwords.

**API key format:** `argus_sk_<43 base64url chars>` (52 chars total). Prefix stored for display: first 17 chars (`argus_sk_` + 8 chars).

**Backward compatibility:** Both the ingest handler and the baselines handler fall back to `"self-hosted"` when no API key is present in the Authorization header. This keeps the self-hosted Docker use case working without any SDK changes.

**Drift detector:** Unchanged in Plan 2. It still runs only for `"self-hosted"`. Per-project drift detection comes in a later plan.

**New env vars:**
- `JWT_SECRET` — HS256 signing key. Defaults to `"dev-secret-change-in-production"` if unset.
- `ARGUS_BASE_URL` — Used to construct OAuth redirect URIs. Default: `"http://localhost:4000"`.
- `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` — GitHub OAuth app credentials.
- `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` — Google OAuth app credentials.

---

## Task 1: JWT package — issue and validate tokens

**Files:**
- Create: `server/internal/auth/jwt.go`
- Create: `server/internal/auth/jwt_test.go`

- [ ] **Step 1: Add the JWT dependency**

```bash
cd server && go get github.com/golang-jwt/jwt/v5
```

Expected: `go.mod` updated with `github.com/golang-jwt/jwt/v5`.

- [ ] **Step 2: Write the failing test**

Create `server/internal/auth/jwt_test.go`:

```go
package auth_test

import (
	"os"
	"testing"

	"github.com/whozpj/argus/server/internal/auth"
)

func TestJWT_RoundTrip(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-1234")
	tok, err := auth.IssueToken("user-123")
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	uid, err := auth.ValidateToken(tok)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if uid != "user-123" {
		t.Errorf("uid = %q, want user-123", uid)
	}
}

func TestJWT_InvalidToken(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-1234")
	_, err := auth.ValidateToken("not.a.token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestJWT_TamperedToken(t *testing.T) {
	os.Setenv("JWT_SECRET", "test-secret-1234")
	tok, _ := auth.IssueToken("user-456")
	tok = tok[:len(tok)-1] + "X"
	_, err := auth.ValidateToken(tok)
	if err == nil {
		t.Fatal("expected error for tampered token")
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

```bash
cd server && go test ./internal/auth/ -run TestJWT -v
```

Expected: compile error — package `auth` does not exist yet.

- [ ] **Step 4: Implement jwt.go**

Create `server/internal/auth/jwt.go`:

```go
package auth

import (
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenTTL = 30 * 24 * time.Hour

type argusClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"uid"`
}

func jwtSecret() []byte {
	s := os.Getenv("JWT_SECRET")
	if s == "" {
		s = "dev-secret-change-in-production"
	}
	return []byte(s)
}

// IssueToken returns a signed HS256 JWT for the given userID, valid for 30 days.
func IssueToken(userID string) (string, error) {
	c := argusClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "argus",
		},
		UserID: userID,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(jwtSecret())
}

// ValidateToken parses and validates a JWT string, returning the userID claim.
func ValidateToken(tokenStr string) (string, error) {
	c := &argusClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(), nil
	})
	if err != nil || !tok.Valid {
		return "", fmt.Errorf("invalid token")
	}
	return c.UserID, nil
}
```

- [ ] **Step 5: Run the tests and verify they pass**

```bash
cd server && go test ./internal/auth/ -run TestJWT -v
```

Expected:
```
--- PASS: TestJWT_RoundTrip
--- PASS: TestJWT_InvalidToken
--- PASS: TestJWT_TamperedToken
PASS
```

- [ ] **Step 6: Commit**

```bash
cd server && git add internal/auth/jwt.go internal/auth/jwt_test.go go.mod go.sum
git commit -m "feat(auth): add JWT issue/validate package"
```

---

## Task 2: Middleware — context keys, RequireJWT, ResolveProject, API key helpers

**Files:**
- Create: `server/internal/auth/middleware.go`
- Create: `server/internal/auth/testhelper_test.go`
- Create: `server/internal/auth/middleware_test.go`

- [ ] **Step 1: Write the failing tests**

Create `server/internal/auth/testhelper_test.go`:

```go
package auth_test

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/whozpj/argus/server/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("argus_test"),
		postgres.WithUsername("argus"),
		postgres.WithPassword("argus"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}
```

Create `server/internal/auth/middleware_test.go`:

```go
package auth_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/whozpj/argus/server/internal/auth"
)

func TestRequireJWT_MissingToken(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := auth.RequireJWT(next)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRequireJWT_ValidBearerToken(t *testing.T) {
	os.Setenv("JWT_SECRET", "mw-test-secret")
	tok, _ := auth.IssueToken("user-abc")

	var gotUID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUID, _ = auth.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := auth.RequireJWT(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if gotUID != "user-abc" {
		t.Errorf("userID = %q, want user-abc", gotUID)
	}
}

func TestRequireJWT_ValidCookie(t *testing.T) {
	os.Setenv("JWT_SECRET", "mw-test-secret")
	tok, _ := auth.IssueToken("user-cookie")

	var gotUID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUID, _ = auth.UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := auth.RequireJWT(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "argus_token", Value: tok})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if gotUID != "user-cookie" {
		t.Errorf("userID = %q, want user-cookie", gotUID)
	}
}

func TestRequireJWT_InvalidToken(t *testing.T) {
	os.Setenv("JWT_SECRET", "mw-test-secret")
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := auth.RequireJWT(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestResolveProject_NoAuth_FallsBackToSelfHosted(t *testing.T) {
	db := newTestDB(t)
	var gotProjectID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProjectID, _ = auth.ProjectIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := auth.ResolveProject(db)(next)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/", nil))

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if gotProjectID != "self-hosted" {
		t.Errorf("projectID = %q, want self-hosted", gotProjectID)
	}
}

func TestResolveProject_ValidAPIKey(t *testing.T) {
	db := newTestDB(t)
	userID, _ := db.UpsertUser("test@example.com", "gh-42", "")
	proj, _ := db.CreateProject(userID, "test-proj")
	rawKey, hash, prefix, _ := auth.GenerateAPIKey()
	db.CreateAPIKey(proj.ID, hash, prefix, "test-key") //nolint:errcheck

	var gotProjectID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProjectID, _ = auth.ProjectIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := auth.ResolveProject(db)(next)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if gotProjectID != proj.ID {
		t.Errorf("projectID = %q, want %q", gotProjectID, proj.ID)
	}
}

func TestResolveProject_InvalidAPIKey(t *testing.T) {
	db := newTestDB(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := auth.ResolveProject(db)(next)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set("Authorization", "Bearer argus_sk_doesnotexist1234567890")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestGenerateAPIKey_Format(t *testing.T) {
	rawKey, hash, prefix, err := auth.GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}
	if len(rawKey) < 20 {
		t.Errorf("rawKey too short: %q", rawKey)
	}
	if rawKey[:9] != "argus_sk_" {
		t.Errorf("rawKey must start with argus_sk_: %q", rawKey)
	}
	if prefix != rawKey[:17] {
		t.Errorf("prefix = %q, want %q", prefix, rawKey[:17])
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64 (SHA-256 hex)", len(hash))
	}
	// Verify HashAPIKey produces the same hash
	if auth.HashAPIKey(rawKey) != hash {
		t.Error("HashAPIKey(rawKey) != hash from GenerateAPIKey")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd server && go test ./internal/auth/ -v 2>&1 | head -20
```

Expected: compile error — `RequireJWT`, `ResolveProject`, etc. not defined.

- [ ] **Step 3: Implement middleware.go**

Create `server/internal/auth/middleware.go`:

```go
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	"github.com/whozpj/argus/server/internal/store"
)

type contextKey int

const (
	contextKeyUserID    contextKey = iota
	contextKeyProjectID contextKey = iota
)

// UserIDFromContext returns the authenticated userID injected by RequireJWT.
func UserIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(contextKeyUserID).(string)
	return v, ok && v != ""
}

// ProjectIDFromContext returns the projectID injected by ResolveProject.
func ProjectIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(contextKeyProjectID).(string)
	return v, ok && v != ""
}

// RequireJWT is middleware that validates a JWT from the "argus_token" cookie or
// "Authorization: Bearer <token>" header (non-API-key tokens only).
// On success it injects the userID into the request context.
func RequireJWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := jwtFromRequest(r)
		if tok == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := ValidateToken(tok)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), contextKeyUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// jwtFromRequest extracts a JWT string from the Authorization header or argus_token cookie.
// Returns "" if the Authorization header contains an API key (starts with "argus_sk_").
func jwtFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		candidate := strings.TrimPrefix(h, "Bearer ")
		if !strings.HasPrefix(candidate, "argus_sk_") {
			return candidate
		}
		return "" // It's an API key, not a JWT
	}
	if c, err := r.Cookie("argus_token"); err == nil {
		return c.Value
	}
	return ""
}

// ResolveProject is middleware that resolves the projectID from an API key.
// If "Authorization: Bearer argus_sk_<key>" is present, it validates the key and
// injects the projectID into context. If absent, it injects "self-hosted" for
// backward compatibility with unauthenticated self-hosted users.
// Returns 401 only when an API key is presented but invalid.
func ResolveProject(db *store.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := "self-hosted"
			if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer argus_sk_") {
				rawKey := strings.TrimPrefix(h, "Bearer ")
				hash := HashAPIKey(rawKey)
				pid, ok, err := db.ResolveAPIKey(hash)
				if err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				if !ok {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				projectID = pid
			}
			ctx := context.WithValue(r.Context(), contextKeyProjectID, projectID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// HashAPIKey returns the SHA-256 hex digest of the raw API key.
// SHA-256 is used instead of bcrypt because API keys are long random strings
// (256-bit entropy), not user-chosen passwords. This keeps ingest latency low.
func HashAPIKey(rawKey string) string {
	h := sha256.Sum256([]byte(rawKey))
	return fmt.Sprintf("%x", h)
}

// GenerateAPIKey creates a new random API key.
// Returns rawKey (full key, shown once), hash (SHA-256, stored in DB),
// and prefix (first 17 chars, for display in key listings).
func GenerateAPIKey() (rawKey, hash, prefix string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	encoded := base64.RawURLEncoding.EncodeToString(b) // 43 chars
	rawKey = "argus_sk_" + encoded                     // 52 chars
	prefix = rawKey[:17]                               // "argus_sk_" + 8 chars
	h := sha256.Sum256([]byte(rawKey))
	hash = fmt.Sprintf("%x", h)
	return
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd server && go test ./internal/auth/ -v -timeout 120s
```

Expected: all 10 middleware + JWT tests pass.

- [ ] **Step 5: Commit**

```bash
cd server && git add internal/auth/middleware.go internal/auth/testhelper_test.go internal/auth/middleware_test.go
git commit -m "feat(auth): add JWT middleware, ResolveProject, API key helpers"
```

---

## Task 3: GitHub OAuth helpers

**Files:**
- Create: `server/internal/auth/oauth_github.go`
- Create: `server/internal/auth/oauth_github_test.go`

- [ ] **Step 1: Write the failing tests**

Create `server/internal/auth/oauth_github_test.go`:

```go
package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whozpj/argus/server/internal/auth"
)

func TestGitHubOAuth_AuthURL(t *testing.T) {
	g := auth.NewGitHubOAuth("my-client-id", "secret")
	got := g.AuthURL("state123", "http://localhost:4000/auth/github/callback")
	if !strings.Contains(got, "client_id=my-client-id") {
		t.Errorf("AuthURL missing client_id: %s", got)
	}
	if !strings.Contains(got, "state=state123") {
		t.Errorf("AuthURL missing state: %s", got)
	}
	if !strings.Contains(got, "github.com") {
		t.Errorf("AuthURL should point to github.com: %s", got)
	}
}

func TestGitHubOAuth_ExchangeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "ghp_testtoken"}) //nolint:errcheck
	}))
	defer srv.Close()

	g := auth.NewGitHubOAuth("id", "secret")
	g.BaseURL = srv.URL // override for test

	tok, err := g.ExchangeCode(context.Background(), "code123")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tok != "ghp_testtoken" {
		t.Errorf("token = %q, want ghp_testtoken", tok)
	}
}

func TestGitHubOAuth_ExchangeCode_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"error": "bad_verification_code"}) //nolint:errcheck
	}))
	defer srv.Close()

	g := auth.NewGitHubOAuth("id", "secret")
	g.BaseURL = srv.URL

	_, err := g.ExchangeCode(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected error for bad code")
	}
}

func TestGitHubOAuth_GetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"id":    42,
			"login": "octocat",
			"email": "octocat@github.com",
		})
	}))
	defer srv.Close()

	g := auth.NewGitHubOAuth("id", "secret")
	g.APIURL = srv.URL

	u, err := g.GetUser(context.Background(), "ghp_test")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Email != "octocat@github.com" {
		t.Errorf("email = %q, want octocat@github.com", u.Email)
	}
	if u.ID != 42 {
		t.Errorf("id = %d, want 42", u.ID)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd server && go test ./internal/auth/ -run TestGitHub -v 2>&1 | head -10
```

Expected: compile error — `NewGitHubOAuth` not defined.

- [ ] **Step 3: Implement oauth_github.go**

Create `server/internal/auth/oauth_github.go`:

```go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GitHubOAuth handles the GitHub OAuth 2.0 authorization code flow.
// BaseURL and APIURL can be overridden in tests to point at httptest servers.
type GitHubOAuth struct {
	ClientID     string
	ClientSecret string
	BaseURL      string // default "https://github.com"
	APIURL       string // default "https://api.github.com"
}

// NewGitHubOAuth returns a GitHubOAuth configured for production.
func NewGitHubOAuth(clientID, clientSecret string) *GitHubOAuth {
	return &GitHubOAuth{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		BaseURL:      "https://github.com",
		APIURL:       "https://api.github.com",
	}
}

// GitHubUser is the subset of GitHub's /user response needed for auth.
type GitHubUser struct {
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Email string `json:"email"`
}

// AuthURL returns the GitHub OAuth authorization URL.
func (g *GitHubOAuth) AuthURL(state, redirectURI string) string {
	return fmt.Sprintf(
		"%s/login/oauth/authorize?client_id=%s&redirect_uri=%s&state=%s&scope=user:email",
		g.BaseURL, g.ClientID, url.QueryEscape(redirectURI), url.QueryEscape(state),
	)
}

// ExchangeCode exchanges an authorization code for a GitHub access token.
func (g *GitHubOAuth) ExchangeCode(ctx context.Context, code string) (string, error) {
	vals := url.Values{
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"code":          {code},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		g.BaseURL+"/login/oauth/access_token",
		strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("github token exchange: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("github token decode: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("github oauth error: %s", result.Error)
	}
	return result.AccessToken, nil
}

// GetUser fetches the authenticated GitHub user using an access token.
func (g *GitHubOAuth) GetUser(ctx context.Context, accessToken string) (GitHubUser, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, g.APIURL+"/user", nil)
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return GitHubUser{}, fmt.Errorf("github get user: %w", err)
	}
	defer resp.Body.Close()

	var u GitHubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return GitHubUser{}, fmt.Errorf("github user decode: %w", err)
	}
	return u, nil
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd server && go test ./internal/auth/ -run TestGitHub -v
```

Expected:
```
--- PASS: TestGitHubOAuth_AuthURL
--- PASS: TestGitHubOAuth_ExchangeCode
--- PASS: TestGitHubOAuth_ExchangeCode_Error
--- PASS: TestGitHubOAuth_GetUser
PASS
```

- [ ] **Step 5: Commit**

```bash
cd server && git add internal/auth/oauth_github.go internal/auth/oauth_github_test.go
git commit -m "feat(auth): add GitHub OAuth helpers"
```

---

## Task 4: Google OAuth helpers

**Files:**
- Create: `server/internal/auth/oauth_google.go`
- Create: `server/internal/auth/oauth_google_test.go`

- [ ] **Step 1: Write the failing tests**

Create `server/internal/auth/oauth_google_test.go`:

```go
package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whozpj/argus/server/internal/auth"
)

func TestGoogleOAuth_LoginURL(t *testing.T) {
	g := auth.NewGoogleOAuth("google-client-id", "secret")
	got := g.LoginURL("state456", "http://localhost:4000/auth/google/callback")
	if !strings.Contains(got, "client_id=google-client-id") {
		t.Errorf("LoginURL missing client_id: %s", got)
	}
	if !strings.Contains(got, "state=state456") {
		t.Errorf("LoginURL missing state: %s", got)
	}
	if !strings.Contains(got, "openid") {
		t.Errorf("LoginURL missing openid scope: %s", got)
	}
}

func TestGoogleOAuth_ExchangeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"access_token": "goog_testtoken"}) //nolint:errcheck
	}))
	defer srv.Close()

	g := auth.NewGoogleOAuth("id", "secret")
	g.TokenURL = srv.URL

	tok, err := g.ExchangeCode(context.Background(), "code789", "http://localhost/callback")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if tok != "goog_testtoken" {
		t.Errorf("token = %q, want goog_testtoken", tok)
	}
}

func TestGoogleOAuth_GetUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"sub":   "google-uid-999",
			"email": "user@gmail.com",
		})
	}))
	defer srv.Close()

	g := auth.NewGoogleOAuth("id", "secret")
	g.UserInfoURL = srv.URL

	u, err := g.GetUser(context.Background(), "goog_test")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if u.Email != "user@gmail.com" {
		t.Errorf("email = %q, want user@gmail.com", u.Email)
	}
	if u.Sub != "google-uid-999" {
		t.Errorf("sub = %q, want google-uid-999", u.Sub)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd server && go test ./internal/auth/ -run TestGoogle -v 2>&1 | head -10
```

Expected: compile error — `NewGoogleOAuth` not defined.

- [ ] **Step 3: Implement oauth_google.go**

Create `server/internal/auth/oauth_google.go`:

```go
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GoogleOAuth handles the Google OAuth 2.0 authorization code flow.
// TokenURL and UserInfoURL can be overridden in tests.
type GoogleOAuth struct {
	ClientID     string
	ClientSecret string
	AuthURL     string // "https://accounts.google.com/o/oauth2/v2/auth"
	TokenURL    string // "https://oauth2.googleapis.com/token"
	UserInfoURL string // "https://www.googleapis.com/oauth2/v3/userinfo"
}

// NewGoogleOAuth returns a GoogleOAuth configured for production.
func NewGoogleOAuth(clientID, clientSecret string) *GoogleOAuth {
	return &GoogleOAuth{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:    "https://oauth2.googleapis.com/token",
		UserInfoURL: "https://www.googleapis.com/oauth2/v3/userinfo",
	}
}

// GoogleUser is the subset of Google's userinfo response needed for auth.
type GoogleUser struct {
	Sub   string `json:"sub"`   // Google's unique user ID
	Email string `json:"email"`
}

// LoginURL returns the Google OAuth authorization URL.
func (g *GoogleOAuth) LoginURL(state, redirectURI string) string {
	return fmt.Sprintf(
		"%s?client_id=%s&redirect_uri=%s&response_type=code&state=%s&scope=openid+email",
		g.AuthURL, g.ClientID, url.QueryEscape(redirectURI), url.QueryEscape(state),
	)
}

// ExchangeCode exchanges an authorization code for a Google access token.
func (g *GoogleOAuth) ExchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	vals := url.Values{
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, g.TokenURL,
		strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("google token exchange: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("google token decode: %w", err)
	}
	if result.Error != "" {
		return "", fmt.Errorf("google oauth error: %s", result.Error)
	}
	return result.AccessToken, nil
}

// GetUser fetches the authenticated Google user using an access token.
func (g *GoogleOAuth) GetUser(ctx context.Context, accessToken string) (GoogleUser, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, g.UserInfoURL, nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return GoogleUser{}, fmt.Errorf("google get user: %w", err)
	}
	defer resp.Body.Close()

	var u GoogleUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return GoogleUser{}, fmt.Errorf("google user decode: %w", err)
	}
	return u, nil
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd server && go test ./internal/auth/ -run TestGoogle -v
```

Expected:
```
--- PASS: TestGoogleOAuth_LoginURL
--- PASS: TestGoogleOAuth_ExchangeCode
--- PASS: TestGoogleOAuth_GetUser
PASS
```

- [ ] **Step 5: Commit**

```bash
cd server && git add internal/auth/oauth_google.go internal/auth/oauth_google_test.go
git commit -m "feat(auth): add Google OAuth helpers"
```

---

## Task 5: Auth HTTP handlers

Routes: `GET /auth/github`, `GET /auth/github/callback`, `GET /auth/google`, `GET /auth/google/callback`, `GET /auth/cli`, `POST /api/v1/auth/token`.

**Files:**
- Create: `server/internal/auth/auth_handlers.go`
- Create: `server/internal/auth/auth_handlers_test.go`

- [ ] **Step 1: Write the failing tests**

Create `server/internal/auth/auth_handlers_test.go`:

```go
package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/whozpj/argus/server/internal/auth"
)

func TestAuthHandlers_GitHubLoginRedirects(t *testing.T) {
	db := newTestDB(t)
	mux := http.NewServeMux()
	auth.RegisterRoutes(mux, db, auth.OAuthConfig{
		BaseURL:          "http://localhost:4000",
		GitHubClientID:   "gh-test-id",
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
		BaseURL:          "http://localhost:4000",
		GoogleClientID:   "goog-test-id",
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
	os.Setenv("JWT_SECRET", "handler-test-secret")
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
	os.Setenv("JWT_SECRET", "handler-test-secret2")
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
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd server && go test ./internal/auth/ -run TestAuthHandlers -v 2>&1 | head -10
```

Expected: compile error — `RegisterRoutes`, `OAuthConfig` not defined.

- [ ] **Step 3: Implement auth_handlers.go**

Create `server/internal/auth/auth_handlers.go`:

```go
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

// issueJWTAndRedirect sets the argus_token cookie and redirects to the dashboard.
// If argus_cli_redirect cookie is present (CLI login flow), creates a session code
// and redirects the browser to the CLI's local callback server instead.
func (h *authHandlers) issueJWTAndRedirect(w http.ResponseWriter, r *http.Request, userID string) {
	tok, err := IssueToken(userID)
	if err != nil {
		slog.Error("issue token", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "argus_token",
		Value:    tok,
		Path:     "/",
		MaxAge:   30 * 24 * 3600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	if c, err := r.Cookie("argus_cli_redirect"); err == nil {
		// Clear the cookie and redirect the CLI
		http.SetCookie(w, &http.Cookie{Name: "argus_cli_redirect", MaxAge: -1, Path: "/"})
		h.createSessionCodeAndRedirect(w, r, userID, c.Value)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
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
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd server && go test ./internal/auth/ -run TestAuthHandlers -v -timeout 120s
```

Expected:
```
--- PASS: TestAuthHandlers_GitHubLoginRedirects
--- PASS: TestAuthHandlers_GoogleLoginRedirects
--- PASS: TestAuthHandlers_TokenExchange_Success
--- PASS: TestAuthHandlers_TokenExchange_InvalidCode
--- PASS: TestAuthHandlers_TokenExchange_CodeConsumedOnce
PASS
```

- [ ] **Step 5: Commit**

```bash
cd server && git add internal/auth/auth_handlers.go internal/auth/auth_handlers_test.go
git commit -m "feat(auth): add OAuth handlers, CLI login, token exchange endpoint"
```

---

## Task 6: Project API handlers

Routes: `GET /api/v1/me`, `POST /api/v1/projects`, `GET /api/v1/projects`, `POST /api/v1/projects/{id}/keys`.

**Files:**
- Create: `server/internal/auth/project_handlers.go`
- Create: `server/internal/auth/project_handlers_test.go`

- [ ] **Step 1: Write the failing tests**

Create `server/internal/auth/project_handlers_test.go`:

```go
package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/whozpj/argus/server/internal/auth"
)

// jwtHeader returns an Authorization header value for the given userID.
func jwtHeader(t *testing.T, userID string) string {
	t.Helper()
	tok, err := auth.IssueToken(userID)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	return "Bearer " + tok
}

func TestProjectHandlers_Me(t *testing.T) {
	os.Setenv("JWT_SECRET", "proj-test-secret")
	db := newTestDB(t)
	userID, err := db.UpsertUser("me@example.com", "gh-me", "")
	if err != nil {
		t.Fatalf("upsert user: %v", err)
	}

	mux := http.NewServeMux()
	auth.RegisterProjectRoutes(mux, db)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req.Header.Set("Authorization", jwtHeader(t, userID))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["email"] != "me@example.com" {
		t.Errorf("email = %v, want me@example.com", resp["email"])
	}
	if resp["id"] != userID {
		t.Errorf("id = %v, want %q", resp["id"], userID)
	}
	// projects should be an empty array, not null
	projs, ok := resp["projects"].([]any)
	if !ok {
		t.Fatalf("projects not an array: %T", resp["projects"])
	}
	if len(projs) != 0 {
		t.Errorf("projects = %v, want []", projs)
	}
}

func TestProjectHandlers_Me_Unauthorized(t *testing.T) {
	db := newTestDB(t)
	mux := http.NewServeMux()
	auth.RegisterProjectRoutes(mux, db)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestProjectHandlers_CreateAndListProjects(t *testing.T) {
	os.Setenv("JWT_SECRET", "proj-test-secret")
	db := newTestDB(t)
	userID, _ := db.UpsertUser("proj@example.com", "gh-proj", "")

	mux := http.NewServeMux()
	auth.RegisterProjectRoutes(mux, db)

	// Create a project
	body := `{"name":"my-api"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(body))
	req.Header.Set("Authorization", jwtHeader(t, userID))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created["name"] != "my-api" {
		t.Errorf("name = %v, want my-api", created["name"])
	}
	projectID, _ := created["id"].(string)
	if projectID == "" {
		t.Fatal("created project has no id")
	}

	// List projects
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req2.Header.Set("Authorization", jwtHeader(t, userID))
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("list: status = %d, want 200", rr2.Code)
	}
	var list []map[string]any
	if err := json.NewDecoder(rr2.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list len = %d, want 1", len(list))
	}
	if list[0]["id"] != projectID {
		t.Errorf("list[0].id = %v, want %q", list[0]["id"], projectID)
	}
}

func TestProjectHandlers_CreateProject_MissingName(t *testing.T) {
	os.Setenv("JWT_SECRET", "proj-test-secret")
	db := newTestDB(t)
	userID, _ := db.UpsertUser("bad@example.com", "gh-bad", "")

	mux := http.NewServeMux()
	auth.RegisterProjectRoutes(mux, db)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{}`))
	req.Header.Set("Authorization", jwtHeader(t, userID))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestProjectHandlers_CreateAPIKey(t *testing.T) {
	os.Setenv("JWT_SECRET", "proj-test-secret")
	db := newTestDB(t)
	userID, _ := db.UpsertUser("keys@example.com", "gh-keys", "")
	proj, _ := db.CreateProject(userID, "prod")

	mux := http.NewServeMux()
	auth.RegisterProjectRoutes(mux, db)

	body := `{"name":"production-key"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+proj.ID+"/keys",
		strings.NewReader(body))
	req.Header.Set("Authorization", jwtHeader(t, userID))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	rawKey, _ := resp["key"].(string)
	if !strings.HasPrefix(rawKey, "argus_sk_") {
		t.Errorf("key = %q, want argus_sk_ prefix", rawKey)
	}
	prefix, _ := resp["prefix"].(string)
	if prefix != rawKey[:17] {
		t.Errorf("prefix = %q, want %q", prefix, rawKey[:17])
	}
	if resp["name"] != "production-key" {
		t.Errorf("name = %v, want production-key", resp["name"])
	}

	// The returned key must actually work — resolve it via the store
	hash := auth.HashAPIKey(rawKey)
	pid, ok, err := db.ResolveAPIKey(hash)
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if !ok {
		t.Fatal("key not found in DB after creation")
	}
	if pid != proj.ID {
		t.Errorf("resolved projectID = %q, want %q", pid, proj.ID)
	}
}

func TestProjectHandlers_CreateAPIKey_WrongProject(t *testing.T) {
	os.Setenv("JWT_SECRET", "proj-test-secret")
	db := newTestDB(t)
	// Two separate users
	userA, _ := db.UpsertUser("userA@example.com", "gh-A", "")
	userB, _ := db.UpsertUser("userB@example.com", "gh-B", "")
	projB, _ := db.CreateProject(userB, "user-b-project")

	mux := http.NewServeMux()
	auth.RegisterProjectRoutes(mux, db)

	// userA tries to create a key for userB's project
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projB.ID+"/keys",
		strings.NewReader(`{"name":"steal"}`))
	req.Header.Set("Authorization", jwtHeader(t, userA))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd server && go test ./internal/auth/ -run TestProjectHandlers -v 2>&1 | head -10
```

Expected: compile error — `RegisterProjectRoutes` not defined.

- [ ] **Step 3: Implement project_handlers.go**

Create `server/internal/auth/project_handlers.go`:

```go
package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/whozpj/argus/server/internal/store"
)

type projectHandlers struct {
	db *store.DB
}

// RegisterProjectRoutes wires project and user management routes onto mux.
// All routes require a valid JWT (enforced by RequireJWT middleware).
func RegisterProjectRoutes(mux *http.ServeMux, db *store.DB) {
	h := &projectHandlers{db: db}
	mux.Handle("GET /api/v1/me", RequireJWT(http.HandlerFunc(h.me)))
	mux.Handle("POST /api/v1/projects", RequireJWT(http.HandlerFunc(h.createProject)))
	mux.Handle("GET /api/v1/projects", RequireJWT(http.HandlerFunc(h.listProjects)))
	mux.Handle("POST /api/v1/projects/{id}/keys", RequireJWT(http.HandlerFunc(h.createKey)))
}

func (h *projectHandlers) me(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	user, err := h.db.GetUserByID(userID)
	if err != nil {
		slog.Error("get user", "err", err)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	projects, err := h.db.ListProjects(userID)
	if err != nil {
		slog.Error("list projects", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type projectJSON struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	type meResponse struct {
		ID       string        `json:"id"`
		Email    string        `json:"email"`
		Projects []projectJSON `json:"projects"`
	}

	resp := meResponse{
		ID:       user.ID,
		Email:    user.Email,
		Projects: make([]projectJSON, 0, len(projects)),
	}
	for _, p := range projects {
		resp.Projects = append(resp.Projects, projectJSON{
			ID:        p.ID,
			Name:      p.Name,
			CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp) //nolint:errcheck
}

func (h *projectHandlers) createProject(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	proj, err := h.db.CreateProject(userID, req.Name)
	if err != nil {
		slog.Error("create project", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"id":         proj.ID,
		"name":       proj.Name,
		"created_at": proj.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *projectHandlers) listProjects(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())

	projects, err := h.db.ListProjects(userID)
	if err != nil {
		slog.Error("list projects", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	type projectJSON struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	out := make([]projectJSON, 0, len(projects))
	for _, p := range projects {
		out = append(out, projectJSON{
			ID:        p.ID,
			Name:      p.Name,
			CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out) //nolint:errcheck
}

func (h *projectHandlers) createKey(w http.ResponseWriter, r *http.Request) {
	userID, _ := UserIDFromContext(r.Context())
	projectID := r.PathValue("id")

	// Verify the authenticated user owns this project.
	projects, err := h.db.ListProjects(userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	owned := false
	for _, p := range projects {
		if p.ID == projectID {
			owned = true
			break
		}
	}
	if !owned {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}

	rawKey, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		slog.Error("generate api key", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	k, err := h.db.CreateAPIKey(projectID, hash, prefix, req.Name)
	if err != nil {
		slog.Error("create api key", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
		"id":         k.ID,
		"key":        rawKey, // Full key returned only once — never stored in plaintext
		"prefix":     prefix,
		"name":       k.Name,
		"created_at": k.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}
```

- [ ] **Step 4: Run all auth tests**

```bash
cd server && go test ./internal/auth/ -v -timeout 180s
```

Expected: all tests pass. Total should be around 25–30 passing tests.

- [ ] **Step 5: Commit**

```bash
cd server && git add internal/auth/project_handlers.go internal/auth/project_handlers_test.go
git commit -m "feat(auth): add project + API key management endpoints"
```

---

## Task 7: Update ingest handler — read projectID from context

Currently the ingest handler hardcodes `"self-hosted"`. After this task it reads projectID from the request context (set by `ResolveProject` middleware in main.go). With no API key header, `ResolveProject` sets projectID to `"self-hosted"` automatically, so existing behavior is preserved.

**Files:**
- Modify: `server/internal/ingest/handler.go`
- Create: `server/internal/ingest/testhelper_test.go`
- Create: `server/internal/ingest/handler_test.go`

- [ ] **Step 1: Write the failing tests**

Create `server/internal/ingest/testhelper_test.go`:

```go
package ingest_test

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/whozpj/argus/server/internal/store"
)

func newTestDB(t *testing.T) *store.DB {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:15-alpine",
		postgres.WithDatabase("argus_test"),
		postgres.WithUsername("argus"),
		postgres.WithPassword("argus"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
		),
	)
	if err != nil {
		t.Fatalf("start postgres container: %v", err)
	}
	t.Cleanup(func() { _ = pgc.Terminate(ctx) })

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get connection string: %v", err)
	}

	db, err := store.Open(dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}
```

Create `server/internal/ingest/handler_test.go`:

```go
package ingest_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/whozpj/argus/server/internal/auth"
	"github.com/whozpj/argus/server/internal/ingest"
)

const validEventBody = `{
	"model":"gpt-4o","provider":"openai","input_tokens":10,
	"output_tokens":50,"latency_ms":200,"finish_reason":"stop",
	"timestamp_utc":"2026-04-09T00:00:00Z"
}`

func TestHandler_NoAPIKey_StoresAsSelfHosted(t *testing.T) {
	db := newTestDB(t)
	// Wrap handler with ResolveProject middleware (as main.go does)
	h := auth.ResolveProject(db)(ingest.NewHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(validEventBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202; body: %s", rr.Code, rr.Body.String())
	}
	count, err := db.EventCount("self-hosted")
	if err != nil {
		t.Fatalf("event count: %v", err)
	}
	if count != 1 {
		t.Errorf("event count = %d, want 1", count)
	}
}

func TestHandler_WithAPIKey_StoresToProject(t *testing.T) {
	db := newTestDB(t)
	// Create user → project → api key
	userID, _ := db.UpsertUser("ingest@example.com", "gh-ingest", "")
	proj, _ := db.CreateProject(userID, "my-proj")
	rawKey, hash, prefix, _ := auth.GenerateAPIKey()
	db.CreateAPIKey(proj.ID, hash, prefix, "ingest-key") //nolint:errcheck

	h := auth.ResolveProject(db)(ingest.NewHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(validEventBody))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", rr.Code)
	}
	count, err := db.EventCount(proj.ID)
	if err != nil {
		t.Fatalf("event count: %v", err)
	}
	if count != 1 {
		t.Errorf("event count for project = %d, want 1", count)
	}
	// Nothing stored under self-hosted
	shCount, _ := db.EventCount("self-hosted")
	if shCount != 0 {
		t.Errorf("self-hosted count = %d, want 0", shCount)
	}
}

func TestHandler_InvalidAPIKey_Returns401(t *testing.T) {
	db := newTestDB(t)
	h := auth.ResolveProject(db)(ingest.NewHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", strings.NewReader(validEventBody))
	req.Header.Set("Authorization", "Bearer argus_sk_doesnotexist123456")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestHandler_MissingFields_Returns400(t *testing.T) {
	db := newTestDB(t)
	h := auth.ResolveProject(db)(ingest.NewHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/events",
		strings.NewReader(`{"model":"","provider":""}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd server && go test ./internal/ingest/ -v 2>&1 | head -10
```

Expected: compile errors.

- [ ] **Step 3: Update handler.go to read projectID from context**

Replace the body of `server/internal/ingest/handler.go` with:

```go
package ingest

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/whozpj/argus/server/internal/auth"
	"github.com/whozpj/argus/server/internal/store"
)

type eventRequest struct {
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	LatencyMs    int    `json:"latency_ms"`
	FinishReason string `json:"finish_reason"`
	TimestampUTC string `json:"timestamp_utc"`
}

type Handler struct {
	db *store.DB
}

func NewHandler(db *store.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req eventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Model == "" || req.Provider == "" {
		http.Error(w, "model and provider are required", http.StatusBadRequest)
		return
	}

	// projectID is resolved by ResolveProject middleware; defaults to "self-hosted".
	projectID, ok := auth.ProjectIDFromContext(r.Context())
	if !ok {
		projectID = "self-hosted"
	}

	err := h.db.InsertEvent(store.Event{
		ProjectID:    projectID,
		Model:        req.Model,
		Provider:     req.Provider,
		InputTokens:  req.InputTokens,
		OutputTokens: req.OutputTokens,
		LatencyMs:    req.LatencyMs,
		FinishReason: req.FinishReason,
		TimestampUTC: req.TimestampUTC,
	})
	if err != nil {
		slog.Error("insert event", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.db.UpdateBaseline(projectID, req.Model, req.OutputTokens, req.LatencyMs); err != nil {
		slog.Error("update baseline", "err", err, "model", req.Model)
	}

	slog.Info("event received", "model", req.Model, "project", projectID,
		"output_tokens", req.OutputTokens, "latency_ms", req.LatencyMs)
	w.WriteHeader(http.StatusAccepted)
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd server && go test ./internal/ingest/ -v -timeout 180s
```

Expected:
```
--- PASS: TestHandler_NoAPIKey_StoresAsSelfHosted
--- PASS: TestHandler_WithAPIKey_StoresToProject
--- PASS: TestHandler_InvalidAPIKey_Returns401
--- PASS: TestHandler_MissingFields_Returns400
PASS
```

- [ ] **Step 5: Commit**

```bash
cd server && git add internal/ingest/handler.go internal/ingest/testhelper_test.go internal/ingest/handler_test.go
git commit -m "feat(ingest): resolve projectID from API key context, fall back to self-hosted"
```

---

## Task 8: Update baselines handler — read projectID from context

The baselines handler currently hardcodes `"self-hosted"`. After this task it reads projectID from context (set by `ResolveProject` middleware), falling back to `"self-hosted"` when no API key is present. Existing tests in `api/baselines_test.go` still pass without modification because they don't set an API key, so the handler falls back to `"self-hosted"` and finds the test data inserted under that project.

**Files:**
- Modify: `server/internal/api/baselines.go`

- [ ] **Step 1: Run existing baselines tests to establish baseline**

```bash
cd server && go test ./internal/api/ -v -timeout 120s
```

Expected: all 4 existing tests pass (they'll keep passing after the change).

- [ ] **Step 2: Update baselines.go to read projectID from context**

In `server/internal/api/baselines.go`, add the `auth` import and replace the three hardcoded `"self-hosted"` strings:

The full updated file:

```go
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/whozpj/argus/server/internal/auth"
	"github.com/whozpj/argus/server/internal/store"
)

type baselinesResponse struct {
	TotalEvents int            `json:"total_events"`
	Baselines   []baselineJSON `json:"baselines"`
}

type baselineJSON struct {
	Model              string  `json:"model"`
	Count              int     `json:"count"`
	MeanOutputTokens   float64 `json:"mean_output_tokens"`
	StdDevOutputTokens float64 `json:"stddev_output_tokens"`
	MeanLatencyMs      float64 `json:"mean_latency_ms"`
	StdDevLatencyMs    float64 `json:"stddev_latency_ms"`
	IsReady            bool    `json:"is_ready"`
	DriftScore         float64 `json:"drift_score"`
	DriftAlerted       bool    `json:"drift_alerted"`
	POutputTokens      float64 `json:"p_output_tokens"`
	PLatencyMs         float64 `json:"p_latency_ms"`
}

// NewBaselinesHandler returns a handler for GET /api/v1/baselines.
// The handler reads projectID from context (set by ResolveProject middleware).
// Falls back to "self-hosted" when no API key is provided, preserving backward
// compatibility for unauthenticated self-hosted users.
func NewBaselinesHandler(db *store.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, ok := auth.ProjectIDFromContext(r.Context())
		if !ok {
			projectID = "self-hosted"
		}

		baselines, err := db.ListBaselines(projectID)
		if err != nil {
			slog.Error("api: list baselines", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		total, err := db.EventCount(projectID)
		if err != nil {
			slog.Error("api: event count", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		driftStates, err := db.GetDriftStates(projectID)
		if err != nil {
			slog.Error("api: drift states", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		resp := baselinesResponse{
			TotalEvents: total,
			Baselines:   make([]baselineJSON, 0, len(baselines)),
		}
		for _, b := range baselines {
			row := baselineJSON{
				Model:              b.Model,
				Count:              b.Count,
				MeanOutputTokens:   round2(b.MeanOutputTokens),
				StdDevOutputTokens: round2(b.StdDevOutputTokens),
				MeanLatencyMs:      round2(b.MeanLatencyMs),
				StdDevLatencyMs:    round2(b.StdDevLatencyMs),
				IsReady:            b.IsReady,
			}
			if ds, ok := driftStates[b.Model]; ok {
				row.DriftScore = round2(ds.Score)
				row.DriftAlerted = ds.Alerted
				row.POutputTokens = round2(ds.POutputTokens)
				row.PLatencyMs = round2(ds.PLatencyMs)
			}
			resp.Baselines = append(resp.Baselines, row)
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}

func round2(f float64) float64 {
	return float64(int(f*100+0.5)) / 100
}
```

- [ ] **Step 3: Run existing baselines tests to verify no regressions**

```bash
cd server && go test ./internal/api/ -v -timeout 120s
```

Expected: all 4 existing tests still pass.

- [ ] **Step 4: Commit**

```bash
cd server && git add internal/api/baselines.go
git commit -m "feat(api): resolve projectID from context in baselines handler"
```

---

## Task 9: Wire all routes in main.go and update docs

**Files:**
- Modify: `server/cmd/main.go`
- Modify: `docs/cloud.md`

- [ ] **Step 1: Update main.go to wire new routes**

Replace the entire content of `server/cmd/main.go`:

```go
package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/whozpj/argus/server/internal/alerts"
	"github.com/whozpj/argus/server/internal/api"
	"github.com/whozpj/argus/server/internal/auth"
	"github.com/whozpj/argus/server/internal/drift"
	"github.com/whozpj/argus/server/internal/ingest"
	"github.com/whozpj/argus/server/internal/store"
)

func main() {
	dsn := getenv("POSTGRES_URL", "postgres://argus:argus@localhost:5432/argus?sslmode=disable")
	addr := getenv("ARGUS_ADDR", ":4000")

	db, err := store.Open(dsn)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	var notifier alerts.Notifier = alerts.Noop{}
	if webhook := getenv("ARGUS_SLACK_WEBHOOK", ""); webhook != "" {
		notifier = alerts.NewSlack(webhook)
		slog.Info("slack alerts enabled")
	}
	go drift.New(db, drift.Interval, notifier).Run()

	oauthCfg := auth.OAuthConfig{
		BaseURL:            getenv("ARGUS_BASE_URL", "http://localhost:4000"),
		GitHubClientID:     getenv("GITHUB_CLIENT_ID", ""),
		GitHubClientSecret: getenv("GITHUB_CLIENT_SECRET", ""),
		GoogleClientID:     getenv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getenv("GOOGLE_CLIENT_SECRET", ""),
	}

	mux := http.NewServeMux()

	// Auth routes — OAuth redirects/callbacks, CLI login, token exchange.
	auth.RegisterRoutes(mux, db, oauthCfg)

	// Project and API key management — JWT required.
	auth.RegisterProjectRoutes(mux, db)

	// Ingest — optional API key; resolves projectID, falls back to "self-hosted".
	mux.Handle("POST /api/v1/events", auth.ResolveProject(db)(ingest.NewHandler(db)))

	// Baselines — optional API key; resolves projectID, falls back to "self-hosted".
	mux.Handle("GET /api/v1/baselines", auth.ResolveProject(db)(http.HandlerFunc(api.NewBaselinesHandler(db))))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	slog.Info("argus server starting", "addr", addr, "dsn", dsn)
	if err := http.ListenAndServe(addr, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

- [ ] **Step 2: Verify the server compiles**

```bash
cd server && go build ./cmd/main.go
```

Expected: no errors. A `main` binary appears in the current directory.

```bash
rm -f main  # clean up the binary
```

- [ ] **Step 3: Run the full test suite**

```bash
cd server && go test ./... -timeout 300s
```

Expected output (all packages pass):
```
ok   github.com/whozpj/argus/server/internal/alerts
ok   github.com/whozpj/argus/server/internal/api
ok   github.com/whozpj/argus/server/internal/auth
ok   github.com/whozpj/argus/server/internal/drift
ok   github.com/whozpj/argus/server/internal/ingest
ok   github.com/whozpj/argus/server/internal/store
```

- [ ] **Step 4: Update docs/cloud.md with Plan 2 information**

In `docs/cloud.md`, update the **Environment variables** table and **What's next** section.

Find the existing environment variables table:
```markdown
| Variable | Default | Description |
|---|---|---|
| `POSTGRES_URL` | `postgres://argus:argus@localhost:5432/argus?sslmode=disable` | Postgres connection string |
| `ARGUS_ADDR` | `:4000` | Server listen address |
| `ARGUS_SLACK_WEBHOOK` | _(empty)_ | Slack webhook URL for drift alerts |
```

Replace it with:
```markdown
| Variable | Default | Description |
|---|---|---|
| `POSTGRES_URL` | `postgres://argus:argus@localhost:5432/argus?sslmode=disable` | Postgres connection string |
| `ARGUS_ADDR` | `:4000` | Server listen address |
| `ARGUS_SLACK_WEBHOOK` | _(empty)_ | Slack webhook URL for drift alerts |
| `JWT_SECRET` | `dev-secret-change-in-production` | HS256 signing key — **change in production** |
| `ARGUS_BASE_URL` | `http://localhost:4000` | Public base URL used to construct OAuth redirect URIs |
| `GITHUB_CLIENT_ID` | _(empty)_ | GitHub OAuth app client ID |
| `GITHUB_CLIENT_SECRET` | _(empty)_ | GitHub OAuth app client secret |
| `GOOGLE_CLIENT_ID` | _(empty)_ | Google OAuth app client ID |
| `GOOGLE_CLIENT_SECRET` | _(empty)_ | Google OAuth app client secret |
```

Find the existing **What's next** section that starts with:
```markdown
**Plan 2 — Auth & API Keys**
JWT middleware, GitHub + Google OAuth endpoints (`/auth/github`, `/auth/google`), API key generation and validation, `/api/v1/me`, `/api/v1/projects`, `/api/v1/projects/:id/keys`.
```

Replace it (mark Plan 2 as done and leave Plans 3–5 as-is):
```markdown
**Plan 2 — Auth & API Keys** ✅ Done
JWT middleware, GitHub + Google OAuth (`/auth/github`, `/auth/google`), API key generation + validation, `/api/v1/me`, `/api/v1/projects`, `/api/v1/projects/:id/keys`, `/api/v1/auth/token` (CLI code exchange).

**Plan 3 — SDK + CLI**
`api_key` parameter in `patch()`, `argus login` / `argus status` / `argus projects` CLI commands.

**Plan 4 — Dashboard**
Login page, project selector, per-project dashboard URL.

**Plan 5 — AWS Infrastructure**
Terraform for ECS Fargate, RDS, ALB, S3, CloudFront, Secrets Manager. GitHub Actions CI/CD.
```

- [ ] **Step 5: Commit**

```bash
cd server && git add cmd/main.go
cd .. && git add docs/cloud.md
git commit -m "feat: wire auth routes in main.go; update cloud docs for Plan 2"
```

---

## Verification

After all 9 tasks are complete, run the full suite one final time:

```bash
cd server && go test ./... -timeout 300s -v 2>&1 | tail -20
```

All 6 packages should pass with no failures.

**Manual smoke test** (optional — requires running Postgres):

```bash
cd server
POSTGRES_URL="postgres://argus:argus@localhost:5432/argus?sslmode=disable" go run ./cmd/main.go &
SERVER_PID=$!

# Health check
curl -s http://localhost:4000/healthz  # → 200

# Ingest without API key (self-hosted path)
curl -s -X POST http://localhost:4000/api/v1/events \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o","provider":"openai","input_tokens":10,"output_tokens":50,"latency_ms":200,"finish_reason":"stop","timestamp_utc":"2026-04-09T12:00:00Z"}'
# → 202

# Baselines
curl -s http://localhost:4000/api/v1/baselines | jq .
# → {"total_events":1,"baselines":[...]}

# Project creation (requires JWT — create one for testing)
# See docs/cloud.md for OAuth setup.

kill $SERVER_PID
```
