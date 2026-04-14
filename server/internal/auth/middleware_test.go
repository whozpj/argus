package auth_test

import (
	"net/http"
	"net/http/httptest"
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
	t.Setenv("JWT_SECRET", "mw-test-secret")
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
	t.Setenv("JWT_SECRET", "mw-test-secret")
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
	t.Setenv("JWT_SECRET", "mw-test-secret")
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

func TestCORSMiddleware_SetsHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := auth.CORSMiddleware(next)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods should be set")
	}
}

func TestCORSMiddleware_PreflightReturns204(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := auth.CORSMiddleware(next)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodOptions, "/", nil))

	if rr.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", rr.Code)
	}
}

func TestResolveProject_JWTWithProjectID_ValidOwner(t *testing.T) {
	t.Setenv("JWT_SECRET", "mw-test-secret")
	db := newTestDB(t)
	userID, _ := db.UpsertUser("owner@example.com", "gh-99", "")
	proj, _ := db.CreateProject(userID, "owned-project")
	tok, _ := auth.IssueToken(userID)

	var gotProjectID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProjectID, _ = auth.ProjectIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := auth.ResolveProject(db)(next)
	req := httptest.NewRequest(http.MethodGet, "/?project_id="+proj.ID, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if gotProjectID != proj.ID {
		t.Errorf("projectID = %q, want %q", gotProjectID, proj.ID)
	}
}

func TestResolveProject_JWTWithProjectID_NotOwner(t *testing.T) {
	t.Setenv("JWT_SECRET", "mw-test-secret")
	db := newTestDB(t)
	ownerID, _ := db.UpsertUser("owner@example.com", "gh-100", "")
	proj, _ := db.CreateProject(ownerID, "owners-project")

	attackerID, _ := db.UpsertUser("attacker@example.com", "gh-101", "")
	tok, _ := auth.IssueToken(attackerID)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := auth.ResolveProject(db)(next)
	req := httptest.NewRequest(http.MethodGet, "/?project_id="+proj.ID, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestResolveProject_JWTWithProjectID_InvalidJWT(t *testing.T) {
	t.Setenv("JWT_SECRET", "mw-test-secret")
	db := newTestDB(t)
	ownerID, _ := db.UpsertUser("owner@example.com", "gh-102", "")
	proj, _ := db.CreateProject(ownerID, "some-project")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := auth.ResolveProject(db)(next)
	req := httptest.NewRequest(http.MethodGet, "/?project_id="+proj.ID, nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}
