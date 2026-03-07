package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	OpenAIStartPath    = "/auth/openai/start"
	OpenAICallbackPath = "/auth/openai/callback"
)

// HandlerConfig controls OAuth callback handling behavior.
type HandlerConfig struct {
	StateTTL time.Duration
}

// Handler serves OAuth start/callback endpoints.
type Handler struct {
	provider     Provider
	stateStore   StateStore
	accountStore OAuthAccountStore
	verifier     IDTokenVerifier
	client       *http.Client
	cfg          HandlerConfig
	now          func() time.Time
}

func NewHandler(cfg HandlerConfig, provider Provider, stateStore StateStore, accountStore OAuthAccountStore, verifier IDTokenVerifier) *Handler {
	if cfg.StateTTL <= 0 {
		cfg.StateTTL = 5 * time.Minute
	}
	return &Handler{
		provider:     provider,
		stateStore:   stateStore,
		accountStore: accountStore,
		verifier:     verifier,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		cfg: cfg,
		now: time.Now,
	}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc(OpenAIStartPath, h.StartHandler)
	mux.HandleFunc(OpenAICallbackPath, h.CallbackHandler)
}

func (h *Handler) SetHTTPClient(client *http.Client) {
	if client != nil {
		h.client = client
	}
}

func (h *Handler) StartHandler(w http.ResponseWriter, r *http.Request) {
	state, err := GenerateState()
	if err != nil {
		http.Error(w, "failed to generate state", http.StatusInternalServerError)
		return
	}
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		http.Error(w, "failed to generate pkce", http.StatusInternalServerError)
		return
	}
	nonce, err := GenerateNonce()
	if err != nil {
		http.Error(w, "failed to generate nonce", http.StatusInternalServerError)
		return
	}

	h.stateStore.Save(state, AuthRequestState{
		Provider:     h.provider.Name(),
		CodeVerifier: verifier,
		Nonce:        nonce,
		ReturnTo:     strings.TrimSpace(r.URL.Query().Get("return_to")),
		ExpiresAt:    h.now().Add(h.cfg.StateTTL),
	})

	authURL, err := h.buildAuthURL(state, challenge, nonce)
	if err != nil {
		http.Error(w, "failed to build authorize URL", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (h *Handler) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	if oauthErr := strings.TrimSpace(r.URL.Query().Get("error")); oauthErr != "" {
		http.Error(w, "oauth error: "+oauthErr, http.StatusBadRequest)
		return
	}

	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" {
		http.Error(w, "missing state or code", http.StatusBadRequest)
		return
	}

	requestState, ok := h.stateStore.Pop(state)
	if !ok || h.now().After(requestState.ExpiresAt) {
		http.Error(w, "invalid or expired state", http.StatusBadRequest)
		return
	}

	tokenResp, err := h.exchangeCode(r.Context(), code, requestState.CodeVerifier)
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}

	claims := &IDTokenClaims{}
	if h.provider.OIDCMode() != OIDCModeDisabled {
		if tokenResp.IDToken == "" {
			http.Error(w, "missing id_token", http.StatusBadGateway)
			return
		}
		claims, err = h.verifier.Verify(r.Context(), h.provider, tokenResp.IDToken, requestState.Nonce)
		if err != nil {
			http.Error(w, "id_token verification failed: "+err.Error(), http.StatusUnauthorized)
			return
		}
	}

	expiresAt := time.Time{}
	if tokenResp.ExpiresIn > 0 {
		expiresAt = h.now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	if err := h.accountStore.Upsert(r.Context(), OAuthAccount{
		Provider:       h.provider.Name(),
		ProviderUserID: claims.Subject,
		Email:          claims.Email,
		AccessToken:    tokenResp.AccessToken,
		RefreshToken:   tokenResp.RefreshToken,
		IDToken:        tokenResp.IDToken,
		TokenExpiresAt: expiresAt,
		ClientID:       h.provider.ClientID(),
		TokenEndpoint:  h.provider.TokenEndpoint(),
	}); err != nil {
		http.Error(w, "failed to persist oauth account", http.StatusInternalServerError)
		return
	}

	if requestState.ReturnTo != "" {
		http.Redirect(w, r, requestState.ReturnTo, http.StatusFound)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status":   "ok",
		"provider": h.provider.Name(),
		"subject":  claims.Subject,
	})
}

func (h *Handler) buildAuthURL(state, challenge, nonce string) (string, error) {
	u, err := url.Parse(h.provider.AuthorizationEndpoint())
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", h.provider.ClientID())
	q.Set("redirect_uri", h.provider.RedirectURI())
	q.Set("scope", strings.Join(h.provider.Scopes(), " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	q.Set("nonce", nonce)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
}

func (h *Handler) exchangeCode(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", h.provider.RedirectURI())
	data.Set("client_id", h.provider.ClientID())
	data.Set("code_verifier", verifier)
	if secret := h.provider.ClientSecret(); secret != "" {
		data.Set("client_secret", secret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.provider.TokenEndpoint(), strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token endpoint returned %s", resp.Status)
	}

	var tr tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}
	if tr.AccessToken == "" {
		return nil, errors.New("token response missing access_token")
	}
	if tr.ExpiresIn < 0 {
		return nil, fmt.Errorf("invalid expires_in: %s", strconv.FormatInt(tr.ExpiresIn, 10))
	}
	return &tr, nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
