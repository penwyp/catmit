package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// IDTokenClaims is the normalized claim subset used by catmit.
type IDTokenClaims struct {
	Subject       string
	Issuer        string
	Audience      []string
	Email         string
	EmailVerified bool
	Nonce         string
	ExpiresAt     time.Time
}

// IDTokenVerifier verifies id_token and returns normalized claims.
type IDTokenVerifier interface {
	Verify(ctx context.Context, provider Provider, rawToken string, expectedNonce string) (*IDTokenClaims, error)
}

// NewIDTokenVerifier returns a verifier based on configured mode.
func NewIDTokenVerifier(mode OIDCMode) IDTokenVerifier {
	switch mode {
	case OIDCModeDisabled:
		return disabledVerifier{}
	case OIDCModeStrict:
		return strictVerifier{}
	case OIDCModePlaceholder:
		fallthrough
	default:
		return placeholderVerifier{}
	}
}

type disabledVerifier struct{}

func (disabledVerifier) Verify(_ context.Context, _ Provider, _ string, _ string) (*IDTokenClaims, error) {
	return &IDTokenClaims{}, nil
}

type strictVerifier struct{}

func (strictVerifier) Verify(_ context.Context, _ Provider, _ string, _ string) (*IDTokenClaims, error) {
	return nil, errors.New("strict OIDC verification is not available in this build; use CATMIT_OAUTH_OIDC_MODE=placeholder")
}

type placeholderVerifier struct{}

type rawClaims struct {
	Sub           string      `json:"sub"`
	Iss           string      `json:"iss"`
	Aud           interface{} `json:"aud"`
	Email         string      `json:"email"`
	EmailVerified bool        `json:"email_verified"`
	Nonce         string      `json:"nonce"`
	Exp           int64       `json:"exp"`
}

func (placeholderVerifier) Verify(_ context.Context, provider Provider, rawToken string, expectedNonce string) (*IDTokenClaims, error) {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid id_token format")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode id_token payload: %w", err)
	}

	var c rawClaims
	if err := json.Unmarshal(payload, &c); err != nil {
		return nil, fmt.Errorf("parse id_token payload: %w", err)
	}
	if c.Sub == "" {
		return nil, errors.New("id_token missing sub")
	}
	if c.Iss == "" {
		return nil, errors.New("id_token missing iss")
	}
	if expectedNonce != "" && c.Nonce != expectedNonce {
		return nil, errors.New("id_token nonce mismatch")
	}
	if provider.Issuer() != "" && c.Iss != provider.Issuer() {
		return nil, fmt.Errorf("id_token issuer mismatch: got %q", c.Iss)
	}

	aud := parseAudience(c.Aud)
	if !contains(aud, provider.ClientID()) {
		return nil, errors.New("id_token audience mismatch")
	}

	expiresAt := time.Unix(c.Exp, 0)
	if !expiresAt.IsZero() && time.Now().After(expiresAt) {
		return nil, errors.New("id_token expired")
	}

	return &IDTokenClaims{
		Subject:       c.Sub,
		Issuer:        c.Iss,
		Audience:      aud,
		Email:         c.Email,
		EmailVerified: c.EmailVerified,
		Nonce:         c.Nonce,
		ExpiresAt:     expiresAt,
	}, nil
}

func parseAudience(v interface{}) []string {
	switch t := v.(type) {
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
