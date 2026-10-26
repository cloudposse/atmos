package context

import (
	"sync"
	"time"
)

// DiscoveryCache caches discovery results with TTL.
type DiscoveryCache struct {
	ttl       time.Duration
	result    *DiscoveryResult
	timestamp time.Time
	mu        sync.RWMutex
}

// NewDiscoveryCache creates a new discovery cache.
func NewDiscoveryCache(ttl time.Duration) *DiscoveryCache {
	return &DiscoveryCache{
		ttl: ttl,
	}
}

// Get retrieves cached result if still valid.
func (c *DiscoveryCache) Get() *DiscoveryResult {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if cache is empty.
	if c.result == nil {
		return nil
	}

	// Check if cache has expired.
	if time.Since(c.timestamp) > c.ttl {
		return nil
	}

	return c.result
}

// Set stores a discovery result in cache.
func (c *DiscoveryCache) Set(result *DiscoveryResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.result = result
	c.timestamp = time.Now()
}

// Invalidate clears the cache.
func (c *DiscoveryCache) Invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.result = nil
	c.timestamp = time.Time{}
}
