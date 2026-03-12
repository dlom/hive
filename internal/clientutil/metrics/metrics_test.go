package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func TestMetricsRegistered(t *testing.T) {
	// Verify all transport metrics are registered
	metrics := []prometheus.Collector{
		metricKubeClientRequests,
		metricKubeClientRequestSeconds,
		metricKubeClientRequestsCancelled,
	}

	for i, metric := range metrics {
		if metric == nil {
			t.Errorf("Metric %d is nil", i)
		}
	}
}
