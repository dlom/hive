package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Transport Metrics (existing metrics from controller/utils)
	metricKubeClientRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_kube_client_requests_total",
			Help: "Counter incremented for each kube client request.",
		},
		[]string{"controller", "method", "resource", "remote", "status"},
	)

	metricKubeClientRequestSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_kube_client_request_seconds",
			Help:    "Length of time for kubernetes client requests.",
			Buckets: []float64{0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 120},
		},
		[]string{"controller", "method", "resource", "remote", "status"},
	)

	metricKubeClientRequestsCancelled = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_kube_client_requests_cancelled_total",
			Help: "Counter incremented for each kube client request cancelled.",
		},
		[]string{"controller", "method", "resource", "remote"},
	)

	// Cache Metrics (new)
	cacheHitsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_client_cache_hits_total",
			Help: "Total number of client cache hits.",
		},
		[]string{"package", "controller"},
	)

	cacheMissesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_client_cache_misses_total",
			Help: "Total number of client cache misses.",
		},
		[]string{"package", "controller"},
	)

	cacheSizeGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "hive_client_cache_size",
			Help: "Current size of the client cache.",
		},
		[]string{"package", "controller"},
	)

	cacheEvictionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_client_cache_evictions_total",
			Help: "Total number of cache evictions.",
		},
		[]string{"package", "controller", "reason"},
	)

	// Operation Metrics (new - for resource operations)
	operationDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_resource_operation_duration_seconds",
			Help:    "Duration of resource operations (apply, patch, delete).",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"controller", "operation", "gvk", "result"},
	)

	operationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_resource_operation_total",
			Help: "Total number of resource operations.",
		},
		[]string{"controller", "operation", "gvk", "result"},
	)
)

func init() {
	// Register all metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		metricKubeClientRequests,
		metricKubeClientRequestSeconds,
		metricKubeClientRequestsCancelled,
		cacheHitsTotal,
		cacheMissesTotal,
		cacheSizeGauge,
		cacheEvictionsTotal,
		operationDurationSeconds,
		operationTotal,
	)
}

// RecordCacheHit records a cache hit for the given package and controller.
func RecordCacheHit(pkg, controller string) {
	cacheHitsTotal.WithLabelValues(pkg, controller).Inc()
}

// RecordCacheMiss records a cache miss for the given package and controller.
func RecordCacheMiss(pkg, controller string) {
	cacheMissesTotal.WithLabelValues(pkg, controller).Inc()
}

// RecordCacheSize records the current cache size.
func RecordCacheSize(pkg, controller string, size int) {
	cacheSizeGauge.WithLabelValues(pkg, controller).Set(float64(size))
}

// RecordEviction records a cache eviction with the given reason.
// Reason should be one of: "lru", "ttl", "health", "manual"
func RecordEviction(pkg, controller, reason string) {
	cacheEvictionsTotal.WithLabelValues(pkg, controller, reason).Inc()
}

// RecordOperation records a resource operation with duration and result.
// Operation should be one of: "apply", "patch", "delete"
// Result should be one of: "success", "failure", "conflict", "timeout"
func RecordOperation(controller, operation, gvk, result string, duration float64) {
	operationDurationSeconds.WithLabelValues(controller, operation, gvk, result).Observe(duration)
	operationTotal.WithLabelValues(controller, operation, gvk, result).Inc()
}
