package cache

import (
	"container/list"
	"context"
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
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
	key        string        // cache key
	client     client.Client
	created    time.Time
	lastAccess time.Time
	element    *list.Element // position in LRU list
}

// lruCache implements ClientCache with LRU eviction and TTL expiration.
type lruCache struct {
	mu sync.RWMutex

	// entries maps cache keys to cached clients
	entries map[string]*cacheEntry

	// accessOrder maintains LRU ordering (front = oldest, back = newest)
	accessOrder *list.List

	// Configuration
	maxSize int
	ttl     time.Duration
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
		accessOrder: list.New(),
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
			// Cache hit - update access time
			c.mu.Lock()
			entry.lastAccess = time.Now()
			c.updateAccessOrderLocked(keyStr)
			cacheSize := len(c.entries)
			c.mu.Unlock()

			// Record cache hit metrics
			recordCacheHit(controllerName)
			recordCacheSize(controllerName, cacheSize)

			return entry.client, nil
		}
	}

	// Cache miss - create new client
	// Record cache miss metrics
	recordCacheMiss(controllerName)

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
	newEntry := &cacheEntry{
		key:        keyStr,
		client:     newClient,
		created:    time.Now(),
		lastAccess: time.Now(),
	}
	newEntry.element = c.accessOrder.PushBack(newEntry)
	c.entries[keyStr] = newEntry

	// Record cache size after adding new entry
	recordCacheSize(controllerName, len(c.entries))

	return newClient, nil
}

// evictOldestLocked evicts the least recently used entry.
// Must be called with lock held.
func (c *lruCache) evictOldestLocked(controllerName string) {
	oldest := c.accessOrder.Front()
	if oldest == nil {
		return
	}

	// Front of list is the oldest entry
	entry := oldest.Value.(*cacheEntry)
	c.evictLocked(entry.key, "lru", controllerName)
}

// evictLocked removes an entry from the cache.
// Must be called with lock held.
func (c *lruCache) evictLocked(key string, reason string, controllerName string) {
	entry, exists := c.entries[key]
	if !exists {
		return
	}

	delete(c.entries, key)

	// Record eviction metrics
	recordEviction(controllerName, reason)

	// Remove from access order
	if entry.element != nil {
		c.accessOrder.Remove(entry.element)
	}
}

// updateAccessOrderLocked moves an entry to the end of the access order (most recently used).
// Must be called with lock held.
func (c *lruCache) updateAccessOrderLocked(key string) {
	entry := c.entries[key]
	if entry != nil && entry.element != nil {
		c.accessOrder.MoveToBack(entry.element)
	}
}
