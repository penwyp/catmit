package llm

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/penwyp/catmit/internal/oauth"
)

func TestResolveLLMBearerToken_Priority(t *testing.T) {
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "oauth.db")
	store, err := oauth.NewSQLiteOAuthAccountStore(dbPath)
	if err != nil {
		t.Fatalf("create oauth store: %v", err)
	}
	if err := store.Upsert(ctx, oauth.OAuthAccount{
		Provider:       "openai",
		ProviderUserID: "u1",
		AccessToken:    "oauth-token",
		TokenExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("seed oauth token: %v", err)
	}

	t.Setenv("CATMIT_OAUTH_DB_SQLITE_PATH", dbPath)
	t.Setenv("CATMIT_OAUTH_PROVIDER", "openai")

	t.Run("default apikey first", func(t *testing.T) {
		t.Setenv("CATMIT_AUTH_PREFERENCE", "")
		token, source, err := resolveLLMBearerToken(ctx, "api-key")
		if err != nil {
			t.Fatalf("resolve token: %v", err)
		}
		if token != "api-key" || source != "apikey" {
			t.Fatalf("got token=%q source=%q", token, source)
		}
	})

	t.Run("oauth preferred", func(t *testing.T) {
		t.Setenv("CATMIT_AUTH_PREFERENCE", "oauth")
		token, source, err := resolveLLMBearerToken(ctx, "api-key")
		if err != nil {
			t.Fatalf("resolve token: %v", err)
		}
		if token != "oauth-token" || source != "oauth" {
			t.Fatalf("got token=%q source=%q", token, source)
		}
	})

	t.Run("fallback to oauth when api key empty", func(t *testing.T) {
		t.Setenv("CATMIT_AUTH_PREFERENCE", "apikey")
		token, source, err := resolveLLMBearerToken(ctx, "")
		if err != nil {
			t.Fatalf("resolve token: %v", err)
		}
		if token != "oauth-token" || source != "oauth" {
			t.Fatalf("got token=%q source=%q", token, source)
		}
	})
}

func TestResolveLLMBearerToken_NoCredentials(t *testing.T) {
	t.Setenv("CATMIT_OAUTH_DB_SQLITE_PATH", "")
	t.Setenv("CATMIT_AUTH_PREFERENCE", "apikey")
	token, source, err := resolveLLMBearerToken(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error, got token=%q source=%q", token, source)
	}
}
