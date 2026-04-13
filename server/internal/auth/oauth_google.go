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
