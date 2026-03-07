package llm

import (
	"context"
	"os"
	"strings"

	"github.com/penwyp/catmit/internal/errors"
	"github.com/penwyp/catmit/internal/oauth"
)

const (
	authPrefAPIKey = "apikey"
	authPrefOAuth  = "oauth"
)

// resolveLLMBearerToken resolves bearer token with configurable priority.
// Default priority: API key first, then OAuth token.
func resolveLLMBearerToken(ctx context.Context, explicitAPIKey string) (string, string, error) {
	apiKey := strings.TrimSpace(explicitAPIKey)
	oauthToken, oauthErr := loadOAuthAccessToken(ctx)

	pref := strings.ToLower(strings.TrimSpace(os.Getenv("CATMIT_AUTH_PREFERENCE")))
	if pref == "" {
		pref = authPrefAPIKey
	}

	useAPIThenOAuth := pref != authPrefOAuth
	if useAPIThenOAuth {
		if apiKey != "" {
			return apiKey, "apikey", nil
		}
		if oauthToken != "" {
			return oauthToken, "oauth", nil
		}
	} else {
		if oauthToken != "" {
			return oauthToken, "oauth", nil
		}
		if apiKey != "" {
			return apiKey, "apikey", nil
		}
	}

	if oauthErr != nil {
		return "", "", oauthErr
	}
	return "", "", errors.ErrLLMAPIKey.WithSuggestion("Set CATMIT_LLM_API_KEY or configure OAuth login and CATMIT_OAUTH_DB_SQLITE_PATH")
}

func loadOAuthAccessToken(ctx context.Context) (string, error) {
	sqlitePath := strings.TrimSpace(os.Getenv("CATMIT_OAUTH_DB_SQLITE_PATH"))
	if sqlitePath == "" {
		return "", nil
	}

	provider := strings.TrimSpace(os.Getenv("CATMIT_OAUTH_PROVIDER"))
	if provider == "" {
		provider = "openai"
	}

	store, err := oauth.NewSQLiteOAuthAccountStore(sqlitePath)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeConfig, "failed to open OAuth sqlite store", err)
	}

	account, ok, err := store.GetLatestByProvider(ctx, provider)
	if err != nil {
		return "", errors.Wrap(errors.ErrTypeConfig, "failed to load OAuth account", err)
	}
	if !ok {
		return "", nil
	}
	return strings.TrimSpace(account.AccessToken), nil
}
