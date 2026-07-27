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

// Cache is a generic TTL in-memory cache.
type Cache[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K]entry[V]
	ttl     time.Duration
}

// New returns a new Cache with the given TTL.
func New[K comparable, V any](ttl time.Duration) *Cache[K, V] {
	return &Cache[K, V]{
		entries: make(map[K]entry[V]),
		ttl:     ttl,
	}
}

// SetTTL updates the TTL for future writes. Existing entries are unaffected.
func (c *Cache[K, V]) SetTTL(ttl time.Duration) {
	c.mu.Lock()
	c.ttl = ttl
	c.mu.Unlock()
}

// Get returns the cached value and true if present and unexpired, otherwise zero value and false.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.expiresAt) {
		var zero V
		return zero, false
	}
	return e.value, true
}

// Set stores a value in the cache with the configured TTL.
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	c.entries[key] = entry[V]{value: value, expiresAt: time.Now().Add(c.ttl)}
	c.mu.Unlock()
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
	c.mu.RLock()
	defer c.mu.RUnlock()
	keys := make([]K, 0, len(c.entries))
	for k, e := range c.entries {
		if now.Before(e.expiresAt) {
			keys = append(keys, k)
		}
	}
	return keys
}
