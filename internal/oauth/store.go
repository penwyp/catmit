package oauth

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// OAuthAccount maps a provider identity to a local user account.
type OAuthAccount struct {
	Provider       string
	ProviderUserID string
	Email          string
	AccessToken    string
	RefreshToken   string
	IDToken        string
	TokenExpiresAt time.Time
	ClientID       string
	TokenEndpoint  string
	UpdatedAt      time.Time
}

// OAuthAccountStore stores OAuth-linked accounts.
type OAuthAccountStore interface {
	Upsert(ctx context.Context, account OAuthAccount) error
}

// OAuthAccountLookup allows querying persisted OAuth accounts.
type OAuthAccountLookup interface {
	GetLatestByProvider(ctx context.Context, provider string) (OAuthAccount, bool, error)
}

// MemoryOAuthAccountStore is a minimal in-memory store for MVP.
type MemoryOAuthAccountStore struct {
	mu   sync.Mutex
	data map[string]OAuthAccount
}

func NewMemoryOAuthAccountStore() *MemoryOAuthAccountStore {
	return &MemoryOAuthAccountStore{data: make(map[string]OAuthAccount)}
}

func (s *MemoryOAuthAccountStore) Upsert(_ context.Context, account OAuthAccount) error {
	if account.Provider == "" || account.ProviderUserID == "" {
		return fmt.Errorf("provider and provider_user_id are required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := account.Provider + ":" + account.ProviderUserID
	account.UpdatedAt = time.Now()
	s.data[key] = account
	return nil
}

// Get is test-only helper to inspect inserted data.
func (s *MemoryOAuthAccountStore) Get(provider, providerUserID string) (OAuthAccount, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[provider+":"+providerUserID]
	return v, ok
}

func (s *MemoryOAuthAccountStore) GetLatestByProvider(_ context.Context, provider string) (OAuthAccount, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var latest OAuthAccount
	found := false
	for _, v := range s.data {
		if v.Provider != provider {
			continue
		}
		if !found || v.UpdatedAt.After(latest.UpdatedAt) {
			latest = v
			found = true
		}
	}
	return latest, found, nil
}
