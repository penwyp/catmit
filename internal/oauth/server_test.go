package oauth

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestStartHandlerRedirectsToProvider(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIConfig{
		ClientID:     "client-id",
		ClientSecret: "secret",
		RedirectURL:  "http://127.0.0.1:8085/auth/openai/callback",
		AuthorizeURL: "https://auth.example.com/oauth/authorize",
		TokenURL:     "https://auth.example.com/oauth/token",
		Issuer:       "https://auth.example.com",
		Scopes:       []string{"openid", "email"},
		OIDCMode:     OIDCModePlaceholder,
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	h := NewHandler(
		HandlerConfig{StateTTL: time.Minute},
		provider,
		NewMemoryStateStore(),
		NewMemoryOAuthAccountStore(),
		NewIDTokenVerifier(provider.OIDCMode()),
	)

	req := httptest.NewRequest(http.MethodGet, OpenAIStartPath, nil)
	w := httptest.NewRecorder()
	h.StartHandler(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusFound)
	}

	loc := w.Header().Get("Location")
	if loc == "" {
		t.Fatal("missing redirect location")
	}
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	q := u.Query()
	for _, key := range []string{"state", "code_challenge", "nonce", "client_id"} {
		if strings.TrimSpace(q.Get(key)) == "" {
			t.Fatalf("missing query param %s", key)
		}
	}
}

func TestCallbackHandlerInvalidState(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIConfig{
		ClientID:     "client-id",
		ClientSecret: "secret",
		RedirectURL:  "http://127.0.0.1:8085/auth/openai/callback",
		AuthorizeURL: "https://auth.example.com/oauth/authorize",
		TokenURL:     "https://auth.example.com/oauth/token",
		Issuer:       "https://auth.example.com",
		Scopes:       []string{"openid", "email"},
		OIDCMode:     OIDCModePlaceholder,
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	h := NewHandler(
		HandlerConfig{StateTTL: time.Minute},
		provider,
		NewMemoryStateStore(),
		NewMemoryOAuthAccountStore(),
		NewIDTokenVerifier(provider.OIDCMode()),
	)

	req := httptest.NewRequest(http.MethodGet, OpenAICallbackPath+"?state=missing&code=abc", nil)
	w := httptest.NewRecorder()
	h.CallbackHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestCallbackHandlerSuccessWithPlaceholderOIDC(t *testing.T) {
	now := time.Now()
	nonce := "nonce-123"
	idToken := buildUnsignedJWT(t, map[string]interface{}{
		"sub":            "user-1",
		"iss":            "https://auth.example.com",
		"aud":            "client-id",
		"email":          "user@example.com",
		"email_verified": true,
		"nonce":          nonce,
		"exp":            now.Add(time.Hour).Unix(),
	})

	provider, err := NewOpenAIProvider(OpenAIConfig{
		ClientID:     "client-id",
		ClientSecret: "secret",
		RedirectURL:  "http://127.0.0.1:8085/auth/openai/callback",
		AuthorizeURL: "https://auth.example.com/oauth/authorize",
		TokenURL:     "https://auth.example.com/oauth/token",
		Issuer:       "https://auth.example.com",
		Scopes:       []string{"openid", "email"},
		OIDCMode:     OIDCModePlaceholder,
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	stateStore := NewMemoryStateStore()
	stateStore.Save("s1", AuthRequestState{
		Provider:     "openai",
		CodeVerifier: "verifier-1",
		Nonce:        nonce,
		ExpiresAt:    now.Add(time.Minute),
	})

	accountStore := NewMemoryOAuthAccountStore()
	h := NewHandler(
		HandlerConfig{StateTTL: time.Minute},
		provider,
		stateStore,
		accountStore,
		NewIDTokenVerifier(provider.OIDCMode()),
	)
	h.SetHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", req.Method)
		}
		body := map[string]interface{}{
			"access_token":  "access-1",
			"refresh_token": "refresh-1",
			"id_token":      idToken,
			"expires_in":    3600,
			"token_type":    "Bearer",
		}
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(bytes.NewReader(buf)),
		}, nil
	})})

	req := httptest.NewRequest(http.MethodGet, OpenAICallbackPath+"?state=s1&code=code-1", nil).WithContext(context.Background())
	w := httptest.NewRecorder()
	h.CallbackHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}

	account, ok := accountStore.Get("openai", "user-1")
	if !ok {
		t.Fatalf("oauth account not persisted")
	}
	if account.Email != "user@example.com" {
		t.Fatalf("account email = %s, want user@example.com", account.Email)
	}
}

func buildUnsignedJWT(t *testing.T, payload map[string]interface{}) string {
	t.Helper()
	header := map[string]interface{}{"alg": "none", "typ": "JWT"}
	hb, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	pb, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb) + "."
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
