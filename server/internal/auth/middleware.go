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

// CORSMiddleware adds permissive CORS headers so the dashboard on port 3000 can
// call the API on port 4000 in development. Both origins are collapsed to one
// domain behind a reverse proxy in production.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ResolveProject is middleware that resolves the projectID for ingest and baselines.
//
// Resolution order:
//  1. API key (argus_sk_*) — used by the SDK; resolves to the key's project.
//  2. JWT + ?project_id query param — used by the dashboard; validates ownership.
//  3. No credentials — falls back to "self-hosted" for unauthenticated instances.
//
// Returns 401 only when credentials are presented but invalid/expired.
// Returns 403 when a JWT user requests a project they don't own.
func ResolveProject(db *store.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := "self-hosted"
			authHeader := r.Header.Get("Authorization")

			switch {
			// Path 1: SDK API key.
			case strings.HasPrefix(authHeader, "Bearer argus_sk_"):
				rawKey := strings.TrimPrefix(authHeader, "Bearer ")
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

			// Path 2: Dashboard JWT + explicit project_id query param.
			case strings.HasPrefix(authHeader, "Bearer ") && r.URL.Query().Get("project_id") != "":
				tok := strings.TrimPrefix(authHeader, "Bearer ")
				userID, err := ValidateToken(tok)
				if err != nil {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
				pid := r.URL.Query().Get("project_id")
				owns, err := db.OwnsProject(userID, pid)
				if err != nil {
					http.Error(w, "internal error", http.StatusInternalServerError)
					return
				}
				if !owns {
					http.Error(w, "forbidden", http.StatusForbidden)
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
