package oauth

import (
	"sync"
	"time"
)

// AuthRequestState keeps transient OAuth data between start and callback.
type AuthRequestState struct {
	Provider     string
	CodeVerifier string
	Nonce        string
	ReturnTo     string
	ExpiresAt    time.Time
}

// StateStore persists OAuth request state.
type StateStore interface {
	Save(state string, data AuthRequestState)
	Pop(state string) (AuthRequestState, bool)
}

// MemoryStateStore is an in-memory state store.
type MemoryStateStore struct {
	mu   sync.Mutex
	data map[string]AuthRequestState
}

func NewMemoryStateStore() *MemoryStateStore {
	return &MemoryStateStore{data: make(map[string]AuthRequestState)}
}

func (s *MemoryStateStore) Save(state string, data AuthRequestState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	s.data[state] = data
}

func (s *MemoryStateStore) Pop(state string) (AuthRequestState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupLocked()
	v, ok := s.data[state]
	if ok {
		delete(s.data, state)
	}
	return v, ok
}

func (s *MemoryStateStore) cleanupLocked() {
	now := time.Now()
	for k, v := range s.data {
		if now.After(v.ExpiresAt) {
			delete(s.data, k)
		}
	}
}
