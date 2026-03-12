// Package metrics provides HTTP transport metrics for Kubernetes client requests.
//
// This package instruments REST config transports to automatically track all HTTP requests
// made by Kubernetes clients, labeled by controller name and whether the target is a remote
// cluster. Metrics are recorded for request counts, durations, and cancellations.
//
// The metrics are automatically applied via config.CopyConfigWithMetrics() and do not require
// manual instrumentation in controllers.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Transport Metrics - used by HTTP transport wrappers for tracking client requests.
	// These metrics are recorded automatically by ControllerMetricsTripper when wrapping
	// REST configs with AddControllerMetricsTransportWrapper.
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
)

func init() {
	// Register transport metrics with controller-runtime's registry
	metrics.Registry.MustRegister(
		metricKubeClientRequests,
		metricKubeClientRequestSeconds,
		metricKubeClientRequestsCancelled,
	)
}

// recordRequest records a Kubernetes client request.
func recordRequest(controller, method, resource, remote, status string) {
	metricKubeClientRequests.WithLabelValues(controller, method, resource, remote, status).Inc()
}

// recordRequestDuration records the duration of a Kubernetes client request.
func recordRequestDuration(controller, method, resource, remote, status string, duration float64) {
	metricKubeClientRequestSeconds.WithLabelValues(controller, method, resource, remote, status).Observe(duration)
}

// recordRequestCancelled records a cancelled Kubernetes client request.
func recordRequestCancelled(controller, method, resource, remote string) {
	metricKubeClientRequestsCancelled.WithLabelValues(controller, method, resource, remote).Inc()
}
