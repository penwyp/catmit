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
	defaultOAuthSQLitePath   = "./catmit_oauth.db"
	defaultOAuthProvider     = "openai"
	defaultOpenAITokenURL    = "https://auth.openai.com/oauth/token"
	tokenRefreshGracePeriod  = 30 * time.Second
	refreshMaxAttempts       = 3
	refreshInitialBackoffDur = 300 * time.Millisecond
	oauthSQLitePathEnv       = "CATMIT_OAUTH_DB_SQLITE_PATH"
	openAIClientIDEnv        = "CATMIT_OAUTH_OPENAI_CLIENT_ID"
	openAIClientSecretEnv    = "CATMIT_OAUTH_OPENAI_CLIENT_SECRET"
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
	sqlitePath := oauthSQLitePath()
	if _, err := os.Stat(sqlitePath); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Wrap(errors.ErrTypeConfig, "failed to stat OAuth sqlite store", err)
	}

	store, err := oauth.NewSQLiteOAuthAccountStore(sqlitePath)
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
		if refreshErr != nil {
			if !account.TokenExpiresAt.IsZero() && time.Now().After(account.TokenExpiresAt) {
				return "", errors.Wrap(errors.ErrTypeExternal, "oauth access token refresh failed and token is expired", refreshErr)
			}
		} else {
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

	httpClient := &http.Client{Timeout: 10 * time.Second}
	backoff := refreshInitialBackoffDur
	var lastErr error

	for attempt := 1; attempt <= refreshMaxAttempts; attempt++ {
		updated, retryable, err := refreshAccessTokenOnce(ctx, httpClient, tokenEndpoint, account)
		if err == nil {
			return updated, nil
		}
		lastErr = err
		if !retryable || attempt == refreshMaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return oauth.OAuthAccount{}, ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
		}
	}

	return oauth.OAuthAccount{}, lastErr
}

func refreshAccessTokenOnce(ctx context.Context, httpClient *http.Client, tokenEndpoint string, account oauth.OAuthAccount) (oauth.OAuthAccount, bool, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(account.RefreshToken))
	clientID := strings.TrimSpace(account.ClientID)
	if clientID == "" {
		clientID = strings.TrimSpace(os.Getenv(openAIClientIDEnv))
	}
	if clientID != "" {
		form.Set("client_id", clientID)
	}
	if clientSecret := strings.TrimSpace(os.Getenv(openAIClientSecretEnv)); clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauth.OAuthAccount{}, false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := httpClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return oauth.OAuthAccount{}, false, ctx.Err()
		}
		return oauth.OAuthAccount{}, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		retryable := resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return oauth.OAuthAccount{}, retryable, fmt.Errorf("refresh token endpoint returned %s", resp.Status)
	}

	var tr refreshTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return oauth.OAuthAccount{}, true, err
	}
	if strings.TrimSpace(tr.AccessToken) == "" {
		return oauth.OAuthAccount{}, false, fmt.Errorf("refresh token response missing access_token")
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
	return updated, false, nil
}

func oauthSQLitePath() string {
	if v := strings.TrimSpace(os.Getenv(oauthSQLitePathEnv)); v != "" {
		return v
	}
	return defaultOAuthSQLitePath
}
