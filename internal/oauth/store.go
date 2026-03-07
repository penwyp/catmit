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
	UpdatedAt      time.Time
}

// OAuthAccountStore stores OAuth-linked accounts.
type OAuthAccountStore interface {
	Upsert(ctx context.Context, account OAuthAccount) error
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
