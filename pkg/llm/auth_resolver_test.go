package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/penwyp/catmit/internal/oauth"
)

func withTempWD(t *testing.T) {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
}

func TestResolveLLMBearerToken_Priority(t *testing.T) {
	withTempWD(t)
	ctx := context.Background()

	store, err := oauth.NewSQLiteOAuthAccountStore(filepath.Clean(defaultOAuthSQLitePath))
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

	t.Run("apikey first", func(t *testing.T) {
		token, source, err := resolveLLMBearerToken(ctx, "api-key")
		if err != nil {
			t.Fatalf("resolve token: %v", err)
		}
		if token != "api-key" || source != "apikey" {
			t.Fatalf("got token=%q source=%q", token, source)
		}
	})

	t.Run("fallback to oauth when api key empty", func(t *testing.T) {
		token, source, err := resolveLLMBearerToken(ctx, "")
		if err != nil {
			t.Fatalf("resolve token: %v", err)
		}
		if token != "oauth-token" || source != "oauth" {
			t.Fatalf("got token=%q source=%q", token, source)
		}
	})
}

func TestResolveLLMBearerToken_AutoRefresh(t *testing.T) {
	withTempWD(t)
	ctx := context.Background()

	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Fatalf("grant_type = %q", got)
		}
		if attempts < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "new-access-token",
			"refresh_token": "new-refresh-token",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	store, err := oauth.NewSQLiteOAuthAccountStore(filepath.Clean(defaultOAuthSQLitePath))
	if err != nil {
		t.Fatalf("create oauth store: %v", err)
	}
	if err := store.Upsert(ctx, oauth.OAuthAccount{
		Provider:       "openai",
		ProviderUserID: "u1",
		AccessToken:    "old-token",
		RefreshToken:   "refresh-token",
		TokenExpiresAt: time.Now().Add(-time.Minute),
		TokenEndpoint:  server.URL,
	}); err != nil {
		t.Fatalf("seed oauth token: %v", err)
	}

	token, source, err := resolveLLMBearerToken(ctx, "")
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if token != "new-access-token" || source != "oauth" {
		t.Fatalf("got token=%q source=%q", token, source)
	}
	if attempts != 3 {
		t.Fatalf("refresh attempts = %d, want 3", attempts)
	}
}

func TestResolveLLMBearerToken_NoCredentials(t *testing.T) {
	withTempWD(t)
	token, source, err := resolveLLMBearerToken(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error, got token=%q source=%q", token, source)
	}
}
