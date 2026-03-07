package oauth

import (
	"errors"
	"os"
	"strings"
)

const (
	defaultOpenAIAuthorizeURL = "https://auth.openai.com/oauth/authorize"
	defaultOpenAITokenURL     = "https://auth.openai.com/oauth/token"
	defaultOpenAIIssuer       = "https://auth.openai.com"
)

// OIDCMode controls how id_token verification is handled.
type OIDCMode string

const (
	OIDCModeStrict      OIDCMode = "strict"
	OIDCModePlaceholder OIDCMode = "placeholder"
	OIDCModeDisabled    OIDCMode = "disabled"
)

// OpenAIConfig is the runtime configuration for OpenAI OAuth.
type OpenAIConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string

	AuthorizeURL string
	TokenURL     string
	Issuer       string
	Scopes       []string
	OIDCMode     OIDCMode
}

// LoadOpenAIConfigFromEnv loads OAuth config from environment variables.
func LoadOpenAIConfigFromEnv() OpenAIConfig {
	mode := OIDCMode(strings.TrimSpace(os.Getenv("CATMIT_OAUTH_OIDC_MODE")))
	if mode == "" {
		mode = OIDCModePlaceholder
	}

	scopes := splitScopes(os.Getenv("CATMIT_OAUTH_OPENAI_SCOPES"))
	if len(scopes) == 0 {
		scopes = []string{"openid", "profile", "email"}
	}

	cfg := OpenAIConfig{
		ClientID:     strings.TrimSpace(os.Getenv("CATMIT_OAUTH_OPENAI_CLIENT_ID")),
		ClientSecret: strings.TrimSpace(os.Getenv("CATMIT_OAUTH_OPENAI_CLIENT_SECRET")),
		RedirectURL:  strings.TrimSpace(os.Getenv("CATMIT_OAUTH_OPENAI_REDIRECT_URL")),
		AuthorizeURL: strings.TrimSpace(os.Getenv("CATMIT_OAUTH_OPENAI_AUTHORIZE_URL")),
		TokenURL:     strings.TrimSpace(os.Getenv("CATMIT_OAUTH_OPENAI_TOKEN_URL")),
		Issuer:       strings.TrimSpace(os.Getenv("CATMIT_OAUTH_OPENAI_ISSUER")),
		Scopes:       scopes,
		OIDCMode:     mode,
	}

	if cfg.AuthorizeURL == "" {
		cfg.AuthorizeURL = defaultOpenAIAuthorizeURL
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = defaultOpenAITokenURL
	}
	if cfg.Issuer == "" {
		cfg.Issuer = defaultOpenAIIssuer
	}

	return cfg
}

// Validate validates the OpenAI OAuth config.
func (c OpenAIConfig) Validate() error {
	if c.ClientID == "" {
		return errors.New("missing CATMIT_OAUTH_OPENAI_CLIENT_ID")
	}
	if c.RedirectURL == "" {
		return errors.New("missing CATMIT_OAUTH_OPENAI_REDIRECT_URL")
	}
	switch c.OIDCMode {
	case OIDCModeStrict, OIDCModePlaceholder, OIDCModeDisabled:
	default:
		return errors.New("invalid CATMIT_OAUTH_OIDC_MODE, expected strict|placeholder|disabled")
	}
	return nil
}

func splitScopes(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	fields := strings.FieldsFunc(v, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}
