package oauth

// Provider defines the minimum OAuth/OIDC provider contract used by the handlers.
type Provider interface {
	Name() string
	ClientID() string
	ClientSecret() string
	RedirectURI() string
	AuthorizationEndpoint() string
	TokenEndpoint() string
	Issuer() string
	Scopes() []string
	OIDCMode() OIDCMode
}

// OpenAIProvider implements Provider for OpenAI OAuth.
type OpenAIProvider struct {
	cfg OpenAIConfig
}

func NewOpenAIProvider(cfg OpenAIConfig) (*OpenAIProvider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &OpenAIProvider{cfg: cfg}, nil
}

func (p *OpenAIProvider) Name() string                  { return "openai" }
func (p *OpenAIProvider) ClientID() string              { return p.cfg.ClientID }
func (p *OpenAIProvider) ClientSecret() string          { return p.cfg.ClientSecret }
func (p *OpenAIProvider) RedirectURI() string           { return p.cfg.RedirectURL }
func (p *OpenAIProvider) AuthorizationEndpoint() string { return p.cfg.AuthorizeURL }
func (p *OpenAIProvider) TokenEndpoint() string         { return p.cfg.TokenURL }
func (p *OpenAIProvider) Issuer() string                { return p.cfg.Issuer }
func (p *OpenAIProvider) Scopes() []string              { return p.cfg.Scopes }
func (p *OpenAIProvider) OIDCMode() OIDCMode            { return p.cfg.OIDCMode }
