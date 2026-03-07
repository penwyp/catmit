package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/oauth"
)

const (
	defaultOAuthSQLitePath  = "./catmit_oauth.db"
	defaultOAuthProvider    = "openai"
	defaultOpenAITokenURL   = "https://auth.openai.com/oauth/token"
	tokenRefreshGracePeriod = 30 * time.Second
)

// resolveLLMBearerToken resolves bearer token with fixed priority:
// API key first, then OAuth token.
func resolveLLMBearerToken(ctx context.Context, explicitAPIKey string) (string, string, error) {
	apiKey := strings.TrimSpace(explicitAPIKey)
	if apiKey != "" {
		return apiKey, "apikey", nil
	}

	oauthToken, oauthErr := loadOAuthAccessToken(ctx)
	if oauthErr != nil {
		return "", "", oauthErr
	}
	if oauthToken != "" {
		return oauthToken, "oauth", nil
	}

	return "", "", errors.ErrLLMAPIKey.WithSuggestion("Set CATMIT_LLM_API_KEY or complete OAuth login first")
}

func loadOAuthAccessToken(ctx context.Context) (string, error) {
	if _, err := os.Stat(defaultOAuthSQLitePath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Wrap(errors.ErrTypeConfig, "failed to stat OAuth sqlite store", err)
	}

	store, err := oauth.NewSQLiteOAuthAccountStore(defaultOAuthSQLitePath)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeConfig, "failed to open OAuth sqlite store", err)
	}

	account, ok, err := store.GetLatestByProvider(ctx, defaultOAuthProvider)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeConfig, "failed to load OAuth account", err)
	}
	if !ok {
		return "", nil
	}

	if shouldRefresh(account) && strings.TrimSpace(account.RefreshToken) != "" {
		refreshed, refreshErr := refreshAccessToken(ctx, account)
		if refreshErr == nil {
			if upsertErr := store.Upsert(ctx, refreshed); upsertErr != nil {
				return "", errors.Wrap(errors.ErrTypeConfig, "failed to persist refreshed OAuth token", upsertErr)
			}
			account = refreshed
		}
	}

	return strings.TrimSpace(account.AccessToken), nil
}

func shouldRefresh(account oauth.OAuthAccount) bool {
	if account.TokenExpiresAt.IsZero() {
		return false
	}
	return time.Now().Add(tokenRefreshGracePeriod).After(account.TokenExpiresAt)
}

type refreshTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	IDToken      string `json:"id_token"`
}

func refreshAccessToken(ctx context.Context, account oauth.OAuthAccount) (oauth.OAuthAccount, error) {
	tokenEndpoint := strings.TrimSpace(account.TokenEndpoint)
	if tokenEndpoint == "" {
		tokenEndpoint = defaultOpenAITokenURL
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(account.RefreshToken))
	if clientID := strings.TrimSpace(account.ClientID); clientID != "" {
		form.Set("client_id", clientID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauth.OAuthAccount{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return oauth.OAuthAccount{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauth.OAuthAccount{}, fmt.Errorf("refresh token endpoint returned %s", resp.Status)
	}

	var tr refreshTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return oauth.OAuthAccount{}, err
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return oauth.OAuthAccount{}, fmt.Errorf("refresh token response missing access_token")
	}

	updated := account
	updated.AccessToken = strings.TrimSpace(tr.AccessToken)
	if strings.TrimSpace(tr.RefreshToken) != "" {
		updated.RefreshToken = strings.TrimSpace(tr.RefreshToken)
	}
	if strings.TrimSpace(tr.IDToken) != "" {
		updated.IDToken = strings.TrimSpace(tr.IDToken)
	}
	if tr.ExpiresIn > 0 {
		updated.TokenExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
	return updated, nil
}
