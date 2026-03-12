package clientutil

// This file re-exports commonly used types and functions from subpackages
// for convenient access.

import (
	"github.com/openshift/hive/internal/clientutil/cache"
	"github.com/openshift/hive/internal/clientutil/config"
	"github.com/openshift/hive/internal/clientutil/discovery"
	"github.com/openshift/hive/internal/clientutil/errors"
	"github.com/openshift/hive/internal/clientutil/fieldmanager"
	"github.com/openshift/hive/internal/clientutil/metrics"
)

// Cache types and functions
type (
	// ClientCache provides thread-safe caching of Kubernetes clients.
	ClientCache = cache.ClientCache

	// ClientFactory creates new clients on cache miss.
	ClientFactory = cache.ClientFactory

	// CacheKey uniquely identifies a cached client.
	CacheKey = cache.CacheKey

	// CacheStats contains cache performance metrics.
	CacheStats = cache.CacheStats

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

// Discovery types and functions
type (
	// CachedDiscoveryManager provides in-memory discovery client caching.
	CachedDiscoveryManager = discovery.CachedDiscoveryManager

	// DiscoveryOption configures discovery manager behavior.
	DiscoveryOption = discovery.DiscoveryOption
)

// NewDiscoveryManager creates a new discovery manager.
var NewDiscoveryManager = discovery.NewDiscoveryManager

// Discovery configuration options
var (
	WithDiscoveryTTL = discovery.WithDiscoveryTTL
)

// Error types and functions
type (
	// ClusterError wraps errors with cluster context.
	ClusterError = errors.ClusterError
)

// Error wrapping and predicates
var (
	WrapClusterError       = errors.WrapClusterError
	AsClusterError         = errors.AsClusterError
	IsNotFound             = errors.IsNotFound
	IsAlreadyExists        = errors.IsAlreadyExists
	IsConflict             = errors.IsConflict
	IsTimeout              = errors.IsTimeout
	IsConnectionFailed     = errors.IsConnectionFailed
	IsAuthenticationFailed = errors.IsAuthenticationFailed
	IsInvalidResource      = errors.IsInvalidResource
	IsCanceled             = errors.IsCanceled
)

// Field manager functions
var (
	FieldManagerName       = fieldmanager.FieldManagerName
	FieldManagerNameLegacy = fieldmanager.FieldManagerNameLegacy
)

// Config utilities
var (
	CopyConfigWithMetrics   = config.CopyConfigWithMetrics
	PrepareConfigForClient  = config.PrepareConfigForClient
	ConfigEquals            = config.ConfigEquals
	IsTransportWrapped      = config.IsTransportWrapped
	GetHTTPClient           = config.GetHTTPClient
)

// Metrics functions
var (
	RecordCacheHit  = metrics.RecordCacheHit
	RecordCacheMiss = metrics.RecordCacheMiss
	RecordCacheSize = metrics.RecordCacheSize
	RecordEviction  = metrics.RecordEviction
	RecordOperation = metrics.RecordOperation

	AddControllerMetricsTransportWrapper = metrics.AddControllerMetricsTransportWrapper
)
