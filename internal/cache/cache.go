// Package cache provides a generic TTL-based in-memory cache safe for concurrent use.
package cache

import (
	"sync"
	"time"
)

type entry[V any] struct {
	value     V
	expiresAt time.Time
}

const defaultMaxEntries = 1024

// Cache is a generic TTL in-memory cache.
type Cache[K comparable, V any] struct {
	mu         sync.RWMutex
	entries    map[K]entry[V]
	ttl        time.Duration
	maxEntries int
}

// New returns a new Cache with the given TTL.
func New[K comparable, V any](ttl time.Duration) *Cache[K, V] {
	return NewWithLimit[K, V](ttl, defaultMaxEntries)
}

// NewWithLimit returns a Cache capped at maxEntries. The cap prevents a stream
// of rotated credentials from retaining entries until every TTL expires.
func NewWithLimit[K comparable, V any](ttl time.Duration, maxEntries int) *Cache[K, V] {
	if maxEntries < 1 {
		maxEntries = 1
	}
	return &Cache[K, V]{
		entries:    make(map[K]entry[V]),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

// SetTTL updates the TTL for future writes. Existing entries are unaffected.
func (c *Cache[K, V]) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	c.ttl = ttl
	c.mu.Unlock()
}

// Reset atomically updates the TTL and removes every cached entry.
func (c *Cache[K, V]) Reset(ttl time.Duration) {
	c.mu.Lock()
	c.ttl = ttl
	c.entries = make(map[K]entry[V])
	c.mu.Unlock()
}

// Get returns the cached value and true if present and unexpired, otherwise zero value and false.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		var zero V
		return zero, false
	}
	if !time.Now().Before(e.expiresAt) {
		c.mu.Lock()
		// Do not delete a newer value that replaced the expired entry between
		// the read lock and this write lock.
		if current, exists := c.entries[key]; exists && current.expiresAt.Equal(e.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores a value in the cache with the configured TTL.
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setLocked(key, value, c.ttl)
}

// SetWithTTL stores a value with an explicit TTL. It is useful for short-lived
// negative cache entries without changing the normal successful-result TTL.
func (c *Cache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.setLocked(key, value, ttl)
}

func (c *Cache[K, V]) setLocked(key K, value V, ttl time.Duration) {
	if ttl <= 0 {
		delete(c.entries, key)
		return
	}
	now := time.Now()
	for existingKey, existing := range c.entries {
		if !now.Before(existing.expiresAt) {
			delete(c.entries, existingKey)
		}
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		var oldestKey K
		var oldestExpiry time.Time
		first := true
		for existingKey, existing := range c.entries {
			if first || existing.expiresAt.Before(oldestExpiry) {
				oldestKey = existingKey
				oldestExpiry = existing.expiresAt
				first = false
			}
		}
		if !first {
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = entry[V]{value: value, expiresAt: now.Add(ttl)}
}

// Delete removes a key from the cache.
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Flush removes all entries from the cache.
func (c *Cache[K, V]) Flush() {
	c.mu.Lock()
	c.entries = make(map[K]entry[V])
	c.mu.Unlock()
}

// Keys returns all keys that currently have unexpired entries.
func (c *Cache[K, V]) Keys() []K {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	keys := make([]K, 0, len(c.entries))
	for k, e := range c.entries {
		if now.Before(e.expiresAt) {
			keys = append(keys, k)
		} else {
			delete(c.entries, k)
		}
	}
	return keys
}
