package resource

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// operationDurationSeconds tracks the duration of resource operations.
	operationDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "hive_resource_operation_duration_seconds",
			Help:    "Duration of resource operations (apply, patch, delete).",
			Buckets: []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30},
		},
		[]string{"controller", "operation", "gvk", "result"},
	)

	// operationTotal counts the total number of resource operations.
	operationTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "hive_resource_operation_total",
			Help: "Total number of resource operations.",
		},
		[]string{"controller", "operation", "gvk", "result"},
	)
)

func init() {
	// Register metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		operationDurationSeconds,
		operationTotal,
	)
}

// recordOperation records a resource operation with duration and result.
// Operation should be one of: "apply", "patch", "delete"
// Result should be one of: "success", "failure", "conflict", "timeout"
func recordOperation(controller, operation, gvk, result string, duration float64) {
	operationDurationSeconds.WithLabelValues(controller, operation, gvk, result).Observe(duration)
	operationTotal.WithLabelValues(controller, operation, gvk, result).Inc()
}
