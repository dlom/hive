package discovery

import (
	"sync"
	"time"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
)

// CachedDiscoveryManager provides in-memory discovery client caching.
// This replaces the disk-based discovery cache in pkg/resource/factory_discovery.go
// which had race conditions and unnecessary disk I/O.
type CachedDiscoveryManager interface {
	// NewCachedDiscoveryClient creates or retrieves a cached discovery client.
	NewCachedDiscoveryClient(cfg *rest.Config) (discovery.CachedDiscoveryInterface, error)

	// InvalidateCache invalidates the discovery cache for a specific cluster.
	InvalidateCache(clusterKey string)

	// InvalidateAll invalidates all cached discovery clients.
	InvalidateAll()
}

// inMemoryDiscoveryManager implements CachedDiscoveryManager with in-memory caching.
type inMemoryDiscoveryManager struct {
	mu    sync.RWMutex
	cache map[string]*discoveryEntry
	ttl   time.Duration
}

// discoveryEntry represents a cached discovery client with metadata.
type discoveryEntry struct {
	client  discovery.CachedDiscoveryInterface
	created time.Time
}

// DiscoveryOption is a functional option for configuring the discovery manager.
type DiscoveryOption func(*inMemoryDiscoveryManager)

// WithDiscoveryTTL sets the TTL for discovery cache entries.
// Default: 10 minutes
func WithDiscoveryTTL(duration time.Duration) DiscoveryOption {
	return func(m *inMemoryDiscoveryManager) {
		m.ttl = duration
	}
}

// NewDiscoveryManager creates a new in-memory discovery manager.
func NewDiscoveryManager(opts ...DiscoveryOption) CachedDiscoveryManager {
	m := &inMemoryDiscoveryManager{
		cache: make(map[string]*discoveryEntry),
		ttl:   10 * time.Minute, // Default TTL
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// NewCachedDiscoveryClient creates or retrieves a cached discovery client.
// The cache key is the REST config Host URL.
func (m *inMemoryDiscoveryManager) NewCachedDiscoveryClient(cfg *rest.Config) (discovery.CachedDiscoveryInterface, error) {
	if cfg == nil {
		return nil, nil
	}

	// Use Host as cache key
	cacheKey := cfg.Host

	// Check if we have a valid cached entry
	m.mu.RLock()
	entry, exists := m.cache[cacheKey]
	m.mu.RUnlock()

	if exists {
		// Check if entry is expired
		if time.Since(entry.created) <= m.ttl {
			return entry.client, nil
		}

		// Entry expired, invalidate it
		m.mu.Lock()
		m.invalidateLocked(cacheKey)
		m.mu.Unlock()
	}

	// Create new discovery client
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Wrap with in-memory cache
	cachedClient := memory.NewMemCacheClient(discoveryClient)

	// Store in cache
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if another goroutine created this entry
	if existing, exists := m.cache[cacheKey]; exists && time.Since(existing.created) <= m.ttl {
		return existing.client, nil
	}

	m.cache[cacheKey] = &discoveryEntry{
		client:  cachedClient,
		created: time.Now(),
	}

	return cachedClient, nil
}

// InvalidateCache invalidates the discovery cache for a specific cluster.
func (m *inMemoryDiscoveryManager) InvalidateCache(clusterKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.invalidateLocked(clusterKey)
}

// InvalidateAll invalidates all cached discovery clients.
func (m *inMemoryDiscoveryManager) InvalidateAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Invalidate each entry
	for key := range m.cache {
		m.invalidateLocked(key)
	}

	// Clear the cache
	m.cache = make(map[string]*discoveryEntry)
}

// invalidateLocked invalidates a specific cache entry.
// Must be called with lock held.
func (m *inMemoryDiscoveryManager) invalidateLocked(key string) {
	if entry, exists := m.cache[key]; exists {
		// Invalidate the discovery cache
		entry.client.Invalidate()
		delete(m.cache, key)
	}
}
