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
