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
