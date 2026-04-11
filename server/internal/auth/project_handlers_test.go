package auth_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	t.Setenv("JWT_SECRET", "proj-test-secret")
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
	t.Setenv("JWT_SECRET", "proj-test-secret")
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
	t.Setenv("JWT_SECRET", "proj-test-secret")
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
	t.Setenv("JWT_SECRET", "proj-test-secret")
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
	t.Setenv("JWT_SECRET", "proj-test-secret")
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
