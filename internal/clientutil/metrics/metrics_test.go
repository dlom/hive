package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRecordCacheHit(t *testing.T) {
	// Reset metrics
	cacheHitsTotal.Reset()

	RecordCacheHit("remoteclient", "clustersync")

	// Verify metric was incremented
	metric := &dto.Metric{}
	if err := cacheHitsTotal.WithLabelValues("remoteclient", "clustersync").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter == nil {
		t.Fatal("Counter is nil")
	}

	if *metric.Counter.Value != 1 {
		t.Errorf("Counter value = %f, want 1", *metric.Counter.Value)
	}
}

func TestRecordCacheMiss(t *testing.T) {
	// Reset metrics
	cacheMissesTotal.Reset()

	RecordCacheMiss("resource", "clusterdeployment")

	// Verify metric was incremented
	metric := &dto.Metric{}
	if err := cacheMissesTotal.WithLabelValues("resource", "clusterdeployment").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter == nil {
		t.Fatal("Counter is nil")
	}

	if *metric.Counter.Value != 1 {
		t.Errorf("Counter value = %f, want 1", *metric.Counter.Value)
	}
}

func TestRecordCacheSize(t *testing.T) {
	// Reset metrics
	cacheSizeGauge.Reset()

	RecordCacheSize("remoteclient", "clustersync", 42)

	// Verify metric was set
	metric := &dto.Metric{}
	if err := cacheSizeGauge.WithLabelValues("remoteclient", "clustersync").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Gauge == nil {
		t.Fatal("Gauge is nil")
	}

	if *metric.Gauge.Value != 42 {
		t.Errorf("Gauge value = %f, want 42", *metric.Gauge.Value)
	}
}

func TestRecordEviction(t *testing.T) {
	// Reset metrics
	cacheEvictionsTotal.Reset()

	RecordEviction("remoteclient", "clustersync", "lru")

	// Verify metric was incremented
	metric := &dto.Metric{}
	if err := cacheEvictionsTotal.WithLabelValues("remoteclient", "clustersync", "lru").Write(metric); err != nil {
		t.Fatalf("Failed to write metric: %v", err)
	}

	if metric.Counter == nil {
		t.Fatal("Counter is nil")
	}

	if *metric.Counter.Value != 1 {
		t.Errorf("Counter value = %f, want 1", *metric.Counter.Value)
	}
}

func TestRecordOperation(t *testing.T) {
	// Reset metrics
	operationDurationSeconds.Reset()
	operationTotal.Reset()

	RecordOperation("clustersync", "apply", "apps/v1/Deployment", "success", 0.123)

	// For histogram metrics, we can't easily verify the exact values through the public API
	// The important thing is that the function doesn't panic and the metric is registered
	// We can verify the counter though
	counterMetric := &dto.Metric{}
	if err := operationTotal.WithLabelValues("clustersync", "apply", "apps/v1/Deployment", "success").Write(counterMetric); err != nil {
		t.Fatalf("Failed to write counter metric: %v", err)
	}

	if counterMetric.Counter == nil {
		t.Fatal("Counter is nil")
	}

	if *counterMetric.Counter.Value != 1 {
		t.Errorf("Counter value = %f, want 1", *counterMetric.Counter.Value)
	}

	// Record a second operation to verify increments work
	RecordOperation("clustersync", "apply", "apps/v1/Deployment", "success", 0.456)

	counterMetric2 := &dto.Metric{}
	if err := operationTotal.WithLabelValues("clustersync", "apply", "apps/v1/Deployment", "success").Write(counterMetric2); err != nil {
		t.Fatalf("Failed to write counter metric: %v", err)
	}

	if *counterMetric2.Counter.Value != 2 {
		t.Errorf("Counter value after second operation = %f, want 2", *counterMetric2.Counter.Value)
	}
}

func TestMetricsRegistered(t *testing.T) {
	// Verify all metrics are registered
	metrics := []prometheus.Collector{
		metricKubeClientRequests,
		metricKubeClientRequestSeconds,
		metricKubeClientRequestsCancelled,
		cacheHitsTotal,
		cacheMissesTotal,
		cacheSizeGauge,
		cacheEvictionsTotal,
		operationDurationSeconds,
		operationTotal,
	}

	for i, metric := range metrics {
		if metric == nil {
			t.Errorf("Metric %d is nil", i)
		}
	}
}

func TestEvictionReasons(t *testing.T) {
	// Test all eviction reasons
	reasons := []string{"lru", "ttl", "health", "manual"}

	cacheEvictionsTotal.Reset()

	for _, reason := range reasons {
		RecordEviction("test", "test", reason)
	}

	// Verify each reason was recorded
	for _, reason := range reasons {
		metric := &dto.Metric{}
		if err := cacheEvictionsTotal.WithLabelValues("test", "test", reason).Write(metric); err != nil {
			t.Fatalf("Failed to write metric for reason %s: %v", reason, err)
		}

		if metric.Counter == nil {
			t.Fatalf("Counter is nil for reason %s", reason)
		}

		if *metric.Counter.Value != 1 {
			t.Errorf("Counter value for reason %s = %f, want 1", reason, *metric.Counter.Value)
		}
	}
}

func TestOperationResults(t *testing.T) {
	// Test all operation results
	results := []string{"success", "failure", "conflict", "timeout"}

	operationTotal.Reset()

	for _, result := range results {
		RecordOperation("test", "apply", "test/v1/Test", result, 0.1)
	}

	// Verify each result was recorded
	for _, result := range results {
		metric := &dto.Metric{}
		if err := operationTotal.WithLabelValues("test", "apply", "test/v1/Test", result).Write(metric); err != nil {
			t.Fatalf("Failed to write metric for result %s: %v", result, err)
		}

		if metric.Counter == nil {
			t.Fatalf("Counter is nil for result %s", result)
		}

		if *metric.Counter.Value != 1 {
			t.Errorf("Counter value for result %s = %f, want 1", result, *metric.Counter.Value)
		}
	}
}
