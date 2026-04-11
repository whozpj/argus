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
