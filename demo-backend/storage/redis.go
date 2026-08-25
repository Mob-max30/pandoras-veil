package storage

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
)

type StoredKey struct {
	Handle      string `json:"handle"`
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
}

type StoredPaste struct {
	Ciphertext       string    `json:"ciphertext"`
	ExpiresAt        time.Time `json:"expires_at"`
	BurnAfterReading bool      `json:"burn_after_reading"`
}

// MemoryStore provides in-memory thread-safe storage with TTL and atomic GETDEL
type MemoryStore struct {
	mu     sync.RWMutex
	keys   map[string]StoredKey
	pastes map[string]StoredPaste
}

func NewMemoryStore() *MemoryStore {
	store := &MemoryStore{
		keys:   make(map[string]StoredKey),
		pastes: make(map[string]StoredPaste),
	}
	go store.cleanupExpired()
	return store
}

func (s *MemoryStore) cleanupExpired() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, paste := range s.pastes {
			if !paste.ExpiresAt.IsZero() && now.After(paste.ExpiresAt) {
				delete(s.pastes, id)
			}
		}
		s.mu.Unlock()
	}
}

func (s *MemoryStore) SaveKey(handle, publicKey, fingerprint string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.keys[handle]; exists {
		return ErrConflict
	}
	s.keys[handle] = StoredKey{
		Handle:      handle,
		PublicKey:   publicKey,
		Fingerprint: fingerprint,
	}
	return nil
}

func (s *MemoryStore) GetKey(handle string) (*StoredKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key, exists := s.keys[handle]
	if !exists {
		return nil, ErrNotFound
	}
	return &key, nil
}

func (s *MemoryStore) SavePaste(id, ciphertext string, ttlSeconds int, burnAfterReading bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var expiresAt time.Time
	if ttlSeconds > 0 {
		expiresAt = time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	}

	s.pastes[id] = StoredPaste{
		Ciphertext:       ciphertext,
		ExpiresAt:        expiresAt,
		BurnAfterReading: burnAfterReading,
	}
	return nil
}

func (s *MemoryStore) GetPaste(id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	paste, exists := s.pastes[id]
	if !exists {
		return "", ErrNotFound
	}

	if !paste.ExpiresAt.IsZero() && time.Now().After(paste.ExpiresAt) {
		delete(s.pastes, id)
		return "", ErrNotFound
	}

	if paste.BurnAfterReading {
		delete(s.pastes, id)
	}

	return paste.Ciphertext, nil
}
