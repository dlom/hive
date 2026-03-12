package cache

import (
	"context"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/hive/internal/clientutil/metrics"
)

// ClientFactory is a function that creates a new client.
// It is called when a cache miss occurs.
type ClientFactory func(ctx context.Context) (client.Client, error)

// ClientCache provides thread-safe caching of Kubernetes clients with LRU eviction and TTL expiration.
//
// The cache automatically invalidates entries when CacheKey changes (kubeconfig updates, API URL failover).
// Manual invalidation is not needed and not supported - the cache is self-managing based on key changes.
//
// Cache performance is monitored via Prometheus metrics automatically - no external stats access needed.
type ClientCache interface {
	// Get retrieves a client from the cache or creates a new one using the factory.
	// If the client exists in cache and is not expired, it is returned immediately.
	// Otherwise, the factory is called to create a new client, which is then cached.
	Get(ctx context.Context, key CacheKey, factory ClientFactory) (client.Client, error)
}

// cacheEntry represents a single cached client with metadata.
type cacheEntry struct {
	client     client.Client
	created    time.Time
	lastAccess time.Time
}

// lruCache implements ClientCache with LRU eviction and TTL expiration.
type lruCache struct {
	mu sync.RWMutex

	// entries maps cache keys to cached clients
	entries map[string]*cacheEntry

	// accessOrder maintains LRU ordering (oldest access first)
	accessOrder []string

	// Configuration
	maxSize int
	ttl     time.Duration

	// Statistics
	hits      int64
	misses    int64
	evictions int64
}

// CacheOption is a functional option for configuring the cache.
type CacheOption func(*lruCache)

// WithMaxSize sets the maximum number of clients to cache.
// When the cache exceeds this size, the least recently used entry is evicted.
// Default: 500
func WithMaxSize(size int) CacheOption {
	return func(c *lruCache) {
		c.maxSize = size
	}
}

// WithTTL sets the time-to-live for cache entries.
// Entries older than TTL are evicted on access.
// Default: 10 minutes
func WithTTL(duration time.Duration) CacheOption {
	return func(c *lruCache) {
		c.ttl = duration
	}
}

// NewCache creates a new client cache with the specified options.
func NewCache(opts ...CacheOption) ClientCache {
	c := &lruCache{
		entries:     make(map[string]*cacheEntry),
		accessOrder: make([]string, 0),
		maxSize:     500,              // Default max size
		ttl:         10 * time.Minute, // Default TTL
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Get retrieves a client from the cache or creates a new one.
func (c *lruCache) Get(ctx context.Context, key CacheKey, factory ClientFactory) (client.Client, error) {
	keyStr := key.String()
	controllerName := getControllerName(ctx)

	// Try to get from cache first (read lock)
	c.mu.RLock()
	entry, exists := c.entries[keyStr]
	c.mu.RUnlock()

	if exists {
		// Check if entry is expired
		if time.Since(entry.created) > c.ttl {
			// Entry expired, remove it and create new one
			c.mu.Lock()
			c.evictLocked(keyStr, "ttl", controllerName)
			c.mu.Unlock()
			exists = false
		} else {
			// Cache hit - update access time and statistics
			c.mu.Lock()
			entry.lastAccess = time.Now()
			c.updateAccessOrderLocked(keyStr)
			c.hits++
			cacheSize := len(c.entries)
			c.mu.Unlock()

			// Record cache hit metrics
			metrics.RecordCacheHit(controllerName)
			metrics.RecordCacheSize(controllerName, cacheSize)

			return entry.client, nil
		}
	}

	// Cache miss - create new client
	c.mu.Lock()
	c.misses++
	c.mu.Unlock()

	// Record cache miss metrics
	metrics.RecordCacheMiss(controllerName)

	// Create client outside of lock to avoid blocking cache during network I/O
	newClient, err := factory(ctx)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if another goroutine already created this entry
	if existing, exists := c.entries[keyStr]; exists {
		// Use the existing entry to avoid duplicates
		existing.lastAccess = time.Now()
		c.updateAccessOrderLocked(keyStr)
		return existing.client, nil
	}

	// Check if we need to evict an entry first
	if len(c.entries) >= c.maxSize {
		c.evictOldestLocked(controllerName)
	}

	// Add new entry
	c.entries[keyStr] = &cacheEntry{
		client:     newClient,
		created:    time.Now(),
		lastAccess: time.Now(),
	}
	c.accessOrder = append(c.accessOrder, keyStr)

	// Record cache size after adding new entry
	metrics.RecordCacheSize(controllerName, len(c.entries))

	return newClient, nil
}

// evictOldestLocked evicts the least recently used entry.
// Must be called with lock held.
func (c *lruCache) evictOldestLocked(controllerName string) {
	if len(c.accessOrder) == 0 {
		return
	}

	// First entry in accessOrder is the oldest
	oldestKey := c.accessOrder[0]
	c.evictLocked(oldestKey, "lru", controllerName)
}

// evictLocked removes an entry from the cache.
// Must be called with lock held.
func (c *lruCache) evictLocked(key string, reason string, controllerName string) {
	if _, exists := c.entries[key]; !exists {
		return
	}

	delete(c.entries, key)
	c.evictions++

	// Record eviction metrics
	metrics.RecordEviction(controllerName, reason)

	// Remove from access order
	for i, k := range c.accessOrder {
		if k == key {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}
}

// updateAccessOrderLocked moves a key to the end of the access order (most recently used).
// Must be called with lock held.
func (c *lruCache) updateAccessOrderLocked(key string) {
	// Find and remove the key from its current position
	for i, k := range c.accessOrder {
		if k == key {
			c.accessOrder = append(c.accessOrder[:i], c.accessOrder[i+1:]...)
			break
		}
	}

	// Add to end (most recently used)
	c.accessOrder = append(c.accessOrder, key)
}
