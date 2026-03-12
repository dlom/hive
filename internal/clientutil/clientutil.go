// Package clientutil provides shared infrastructure for Kubernetes client management.
//
// This package contains utilities for:
//   - Client caching with LRU eviction and TTL expiration (cache subpackage)
//   - REST config preparation with metrics and overrides (config subpackage)
//   - Error wrapping with cluster context (errors subpackage)
//   - Field manager naming conventions (fieldmanager subpackage)
//   - HTTP transport metrics (metrics subpackage)
//
// The main clientutil package re-exports commonly used types and functions from
// subpackages for convenient access. Controllers should import clientutil rather
// than reaching into subpackages directly.
//
// # Shared Cache
//
// The shared cache (GetSharedCache) is used exclusively for remote cluster clients
// via pkg/remoteclient.Builder. It provides automatic invalidation on kubeconfig
// changes and optimal memory usage across all controllers.
package clientutil

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

// ============================================================================
// Cache - Client caching with LRU eviction and TTL
// ============================================================================
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

var (
	// WithMaxSize sets the maximum number of cached clients (default: 500).
	WithMaxSize = cache.WithMaxSize

	// WithTTL sets the time-to-live for cache entries (default: 10 minutes).
	WithTTL = cache.WithTTL
)

// ============================================================================
// Errors - Cluster-context error wrapping
// ============================================================================
type (
	// ClusterError wraps errors with cluster context.
	ClusterError = errors.ClusterError
)

var (
	// WrapClusterError wraps errors with cluster and operation context.
	WrapClusterError = errors.WrapClusterError
)

// ============================================================================
// Field Manager - Naming conventions for Server-Side Apply
// ============================================================================

var (
	// FieldManagerName returns the unified field manager name for a controller.
	// Format: "hive-{controller}"
	FieldManagerName = fieldmanager.FieldManagerName
)

// ============================================================================
// Config - REST config preparation and kubeconfig parsing
// ============================================================================

var (
	// CopyConfigWithMetrics returns a config copy with HTTP metrics wrapper.
	CopyConfigWithMetrics = config.CopyConfigWithMetrics

	// PrepareConfigForClient applies URL and IP overrides to a config.
	PrepareConfigForClient = config.PrepareConfigForClient

	// RestConfigFromSecret parses a kubeconfig secret into a REST config.
	RestConfigFromSecret = config.RestConfigFromSecret

	// ValidateKubeconfig validates kubeconfig data for security issues (HIVE-2485).
	ValidateKubeconfig = config.ValidateKubeconfig
)

// ============================================================================
// Shared Cache - Application-wide client cache management
// ============================================================================

// InitializeSharedCache creates the shared client cache with custom options.
//
// This is optional and should be called once during application startup (e.g., in main.go)
// if non-default cache settings are needed. If not called, GetSharedCache() will create
// a default cache on first use with:
//   - Max size: 500 clients
//   - TTL: 10 minutes
//
// Example:
//
//	clientutil.InitializeSharedCache(
//	    clientutil.WithMaxSize(1000),
//	    clientutil.WithTTL(30*time.Minute),
//	)
func InitializeSharedCache(opts ...CacheOption) {
	sharedCacheMu.Lock()
	defer sharedCacheMu.Unlock()
	sharedCache = NewCache(opts...)
}

// GetSharedCache returns a controller-specific view of the shared client cache.
//
// The returned cache is used exclusively for remote cluster clients (via remoteclient.Builder).
// All metrics are automatically tagged with the controller name. Multiple controllers share
// the same underlying cache for optimal memory usage and cache hit rates.
//
// Controllers should call this during initialization and pass the cache to remoteclient.Builder:
//
//	cache := clientutil.GetSharedCache(hivev1.MyControllerName)
//	builder := remoteclient.NewBuilderWithOptions(
//	    remoteclient.WithCache(cache),
//	    remoteclient.WithControllerName(hivev1.MyControllerName),
//	    remoteclient.WithClusterDeployment(c, cd),
//	)
func GetSharedCache(controllerName hivev1.ControllerName) ClientCache {
	realCache := getSharedCacheInstance()
	return cache.NewControllerCache(realCache, string(controllerName))
}

// getSharedCacheInstance returns the underlying shared cache instance.
// If not explicitly initialized via InitializeSharedCache, creates a default cache.
func getSharedCacheInstance() ClientCache {
	sharedCacheOnce.Do(func() {
		// Check if already initialized (lock not needed inside Once.Do)
		if sharedCache == nil {
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
