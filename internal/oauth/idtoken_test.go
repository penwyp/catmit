package oauth

import (
	"context"
	"testing"
	"time"
)

func TestPlaceholderVerifierRejectsNonceMismatch(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIConfig{
		ClientID:     "client-id",
		RedirectURL:  "http://127.0.0.1:8085/auth/openai/callback",
		AuthorizeURL: "https://auth.example.com/oauth/authorize",
		TokenURL:     "https://auth.example.com/oauth/token",
		Issuer:       "https://auth.example.com",
		Scopes:       []string{"openid"},
		OIDCMode:     OIDCModePlaceholder,
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider() error = %v", err)
	}

	token := buildUnsignedJWT(t, map[string]interface{}{
		"sub":   "u1",
		"iss":   "https://auth.example.com",
		"aud":   "client-id",
		"nonce": "n1",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})

	_, err = NewIDTokenVerifier(OIDCModePlaceholder).Verify(context.Background(), provider, token, "n2")
	if err == nil {
		t.Fatalf("expected nonce mismatch error")
	}
}
