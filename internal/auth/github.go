package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type OAuthProvider interface {
	AuthorizeURL(state, redirectURL string) string
	Exchange(ctx context.Context, code, redirectURL string) (OAuthUser, error)
}

type GitHubProviderConfig struct {
	ClientID     string
	ClientSecret string
	HTTPClient   *http.Client
}

type GitHubProvider struct {
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

func NewGitHubProvider(cfg GitHubProviderConfig) (*GitHubProvider, error) {
	if strings.TrimSpace(cfg.ClientID) == "" {
		return nil, fmt.Errorf("github client id is required")
	}
	if strings.TrimSpace(cfg.ClientSecret) == "" {
		return nil, fmt.Errorf("github client secret is required")
	}

	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	return &GitHubProvider{
		clientID:     strings.TrimSpace(cfg.ClientID),
		clientSecret: strings.TrimSpace(cfg.ClientSecret),
		httpClient:   client,
	}, nil
}

func (p *GitHubProvider) AuthorizeURL(state, redirectURL string) string {
	query := url.Values{}
	query.Set("client_id", p.clientID)
	query.Set("redirect_uri", redirectURL)
	query.Set("scope", "read:user user:email")
	query.Set("state", state)
	return "https://github.com/login/oauth/authorize?" + query.Encode()
}

func (p *GitHubProvider) Exchange(ctx context.Context, code, redirectURL string) (OAuthUser, error) {
	tokenValues := url.Values{}
	tokenValues.Set("client_id", p.clientID)
	tokenValues.Set("client_secret", p.clientSecret)
	tokenValues.Set("code", strings.TrimSpace(code))
	tokenValues.Set("redirect_uri", redirectURL)

	tokenReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(tokenValues.Encode()),
	)
	if err != nil {
		return OAuthUser{}, fmt.Errorf("build github token request: %w", err)
	}
	tokenReq.Header.Set("Accept", "application/json")
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenReq.Header.Set("User-Agent", "sortit")

	tokenResp, err := p.httpClient.Do(tokenReq)
	if err != nil {
		return OAuthUser{}, fmt.Errorf("exchange github code: %w", err)
	}
	defer tokenResp.Body.Close() //nolint:errcheck

	if tokenResp.StatusCode != http.StatusOK {
		return OAuthUser{}, fmt.Errorf("github token exchange failed with %d", tokenResp.StatusCode)
	}

	var tokenPayload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenPayload); err != nil {
		return OAuthUser{}, fmt.Errorf("decode github token response: %w", err)
	}
	if strings.TrimSpace(tokenPayload.AccessToken) == "" {
		return OAuthUser{}, fmt.Errorf("github token exchange returned no access token")
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return OAuthUser{}, fmt.Errorf("build github user request: %w", err)
	}
	userReq.Header.Set("Accept", "application/vnd.github+json")
	userReq.Header.Set("Authorization", "Bearer "+tokenPayload.AccessToken)
	userReq.Header.Set("User-Agent", "sortit")

	userResp, err := p.httpClient.Do(userReq)
	if err != nil {
		return OAuthUser{}, fmt.Errorf("load github user: %w", err)
	}
	defer userResp.Body.Close() //nolint:errcheck

	if userResp.StatusCode != http.StatusOK {
		return OAuthUser{}, fmt.Errorf("github user lookup failed with %d", userResp.StatusCode)
	}

	var userPayload struct {
		ID        int64  `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		AvatarURL string `json:"avatar_url"`
		Email     string `json:"email"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&userPayload); err != nil {
		return OAuthUser{}, fmt.Errorf("decode github user response: %w", err)
	}

	email := strings.TrimSpace(userPayload.Email)
	if email == "" {
		email, err = p.lookupPrimaryEmail(ctx, tokenPayload.AccessToken)
		if err != nil {
			return OAuthUser{}, err
		}
	}

	return OAuthUser{
		Provider:       "github",
		ProviderUserID: fmt.Sprintf("%d", userPayload.ID),
		Login:          strings.TrimSpace(userPayload.Login),
		DisplayName:    strings.TrimSpace(userPayload.Name),
		AvatarURL:      strings.TrimSpace(userPayload.AvatarURL),
		Email:          email,
	}, nil
}

func (p *GitHubProvider) lookupPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", fmt.Errorf("build github email request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "sortit")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("load github emails: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github email lookup failed with %d", resp.StatusCode)
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", fmt.Errorf("decode github emails response: %w", err)
	}

	for _, item := range emails {
		if item.Primary && item.Verified {
			return strings.TrimSpace(item.Email), nil
		}
	}
	for _, item := range emails {
		if item.Verified {
			return strings.TrimSpace(item.Email), nil
		}
	}
	return "", nil
}
