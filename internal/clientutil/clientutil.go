package clientutil

// This file re-exports commonly used types and functions from subpackages
// for convenient access.

import (
	"sync"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil/cache"
	"github.com/openshift/hive/internal/clientutil/config"
	"github.com/openshift/hive/internal/clientutil/errors"
	"github.com/openshift/hive/internal/clientutil/fieldmanager"
)

var (
	sharedCache     ClientCache
	sharedCacheMu   sync.RWMutex
	sharedCacheOnce sync.Once
)

// Cache types and functions
type (
	// ClientCache provides thread-safe caching of Kubernetes clients.
	ClientCache = cache.ClientCache

	// ClientFactory creates new clients on cache miss.
	ClientFactory = cache.ClientFactory

	// CacheKey uniquely identifies a cached client.
	CacheKey = cache.CacheKey

	// CacheOption configures cache behavior.
	CacheOption = cache.CacheOption
)

// NewCache creates a new client cache.
var NewCache = cache.NewCache

// NewCacheKey creates a cache key from components.
var NewCacheKey = cache.NewCacheKey

// Cache configuration options
var (
	WithMaxSize = cache.WithMaxSize
	WithTTL     = cache.WithTTL
)

// Error types and functions
type (
	// ClusterError wraps errors with cluster context.
	ClusterError = errors.ClusterError
)

// Error wrapping
var (
	WrapClusterError = errors.WrapClusterError
)

// Field manager functions
var (
	FieldManagerName = fieldmanager.FieldManagerName
)

// Config utilities
var (
	CopyConfigWithMetrics  = config.CopyConfigWithMetrics
	PrepareConfigForClient = config.PrepareConfigForClient
	RestConfigFromSecret   = config.RestConfigFromSecret
	ValidateKubeconfig     = config.ValidateKubeconfig
)

// InitializeSharedCache creates the shared client cache used across all controllers.
// This should be called once during application startup (e.g., in main.go).
// If not called, GetSharedCache() will create a default cache on first use.
func InitializeSharedCache(opts ...CacheOption) {
	sharedCacheMu.Lock()
	defer sharedCacheMu.Unlock()
	sharedCache = NewCache(opts...)
}

// GetSharedCache returns a controller-specific view of the shared client cache.
// The returned cache automatically tags all metrics with the controller name.
//
// If no cache has been initialized, creates a default cache with:
//   - Max size: 500 clients
//   - TTL: 10 minutes
//
// All controllers should use this shared cache for optimal memory usage
// and cache hit rates across the entire application.
func GetSharedCache(controllerName hivev1.ControllerName) ClientCache {
	realCache := getSharedCacheInstance()
	return cache.NewControllerCache(realCache, string(controllerName))
}

// getSharedCacheInstance returns the underlying shared cache instance.
func getSharedCacheInstance() ClientCache {
	sharedCacheOnce.Do(func() {
		sharedCacheMu.Lock()
		defer sharedCacheMu.Unlock()
		if sharedCache == nil {
			// Default configuration if not explicitly initialized
			sharedCache = NewCache(
				WithMaxSize(500),
				WithTTL(10*time.Minute),
			)
		}
	})

	sharedCacheMu.RLock()
	defer sharedCacheMu.RUnlock()
	return sharedCache
}
