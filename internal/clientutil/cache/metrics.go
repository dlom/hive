// This file contains Prometheus metrics for the remote client cache.
// Metrics are colocated with the cache implementation for locality of behavior.
// All metrics are private and automatically recorded by cache operations.
package cache

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// cacheHitsTotal counts cache hits by controller.
	cacheHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_client_cache_hits_total",
			Help: "Total number of client cache hits.",
		},
		[]string{"controller"},
	)

	// cacheMissesTotal counts cache misses by controller.
	cacheMissesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_client_cache_misses_total",
			Help: "Total number of client cache misses.",
		},
		[]string{"controller"},
	)

	// cacheSizeGauge tracks current cache size by controller.
	cacheSizeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hive_client_cache_size",
			Help: "Current size of the client cache.",
		},
		[]string{"controller"},
	)

	// cacheEvictionsTotal counts cache evictions by controller and reason.
	cacheEvictionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_client_cache_evictions_total",
			Help: "Total number of cache evictions.",
		},
		[]string{"controller", "reason"},
	)
)

func init() {
	// Register metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		cacheHitsTotal,
		cacheMissesTotal,
		cacheSizeGauge,
		cacheEvictionsTotal,
	)
}

// recordCacheHit records a cache hit for the given controller.
func recordCacheHit(controller string) {
	cacheHitsTotal.WithLabelValues(controller).Inc()
}

// recordCacheMiss records a cache miss for the given controller.
func recordCacheMiss(controller string) {
	cacheMissesTotal.WithLabelValues(controller).Inc()
}

// recordCacheSize records the current cache size.
func recordCacheSize(controller string, size int) {
	cacheSizeGauge.WithLabelValues(controller).Set(float64(size))
}

// recordEviction records a cache eviction with the given reason.
// Reason should be one of: "lru", "ttl"
func recordEviction(controller, reason string) {
	cacheEvictionsTotal.WithLabelValues(controller, reason).Inc()
}
