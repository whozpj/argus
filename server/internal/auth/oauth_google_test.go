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
