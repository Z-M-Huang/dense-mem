package inmem

import (
	"context"
	"sync"
	"time"
)

type counterEntry struct {
	count     int64
	expiresAt time.Time
}

// InMemoryRateLimitStore is a concurrency-safe, TTL-aware in-memory rate-limit store.
type InMemoryRateLimitStore struct {
	mu   sync.Mutex
	data map[string]counterEntry
	now  func() time.Time
}

// NewInMemoryRateLimitStore creates a new in-memory rate-limit store.
func NewInMemoryRateLimitStore() *InMemoryRateLimitStore {
	return NewInMemoryRateLimitStoreWithClock(time.Now)
}

// NewInMemoryRateLimitStoreWithClock creates a store with a controllable clock for testing.
func NewInMemoryRateLimitStoreWithClock(now func() time.Time) *InMemoryRateLimitStore {
	return &InMemoryRateLimitStore{
		data: make(map[string]counterEntry),
		now:  now,
	}
}

// IncrWithExpire atomically increments the counter for the given key and sets its TTL.
// If the key is expired or absent, it starts from 1.
// Expired entries are lazily deleted before incrementing.
func (s *InMemoryRateLimitStore) IncrWithExpire(ctx context.Context, key string, expireSeconds int64) (int64, error) {
	_ = ctx // unused in in-memory implementation
	now := s.now()

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, exists := s.data[key]

	// Lazily delete expired entries
	if exists && now.After(entry.expiresAt) {
		delete(s.data, key)
		exists = false
	}

	if !exists {
		entry = counterEntry{
			count:     0,
			expiresAt: now.Add(time.Duration(expireSeconds) * time.Second),
		}
	}

	entry.count++
	s.data[key] = entry

	return entry.count, nil
}
