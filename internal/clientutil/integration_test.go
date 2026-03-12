package clientutil_test

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hive/internal/clientutil"
)

// TestIntegration_CacheWithConfigUtils tests cache and config utilities working together.
func TestIntegration_CacheWithConfigUtils(t *testing.T) {
	// Create a cache
	cache := clientutil.NewCache(
		clientutil.WithMaxSize(10),
		clientutil.WithTTL(1*time.Hour),
	)

	// Create a REST config
	cfg := &rest.Config{
		Host:        "https://test-cluster.example.com:6443",
		BearerToken: "test-token",
	}

	// Apply metrics wrapper immutably
	cfgWithMetrics := clientutil.CopyConfigWithMetrics(cfg, hivev1.ClustersyncControllerName, true)

	// Verify original wasn't modified
	if cfg.WrapTransport != nil {
		t.Error("Original config was mutated (wrapper applied)")
	}

	// Verify copy has wrapper
	if cfgWithMetrics.WrapTransport == nil {
		t.Error("Config with metrics should have wrapper")
	}

	// Create cache key
	key := clientutil.NewCacheKey("hive/test-cluster", "v1", cfg.Host)

	// Use cache with factory
	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	ctx := context.Background()

	// First call - cache miss
	client1, err := cache.Get(ctx, key, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if client1 == nil {
		t.Fatal("Get() returned nil client")
	}
	if factoryCalls != 1 {
		t.Errorf("Factory calls = %d, want 1", factoryCalls)
	}

	// Second call - cache hit
	client2, err := cache.Get(ctx, key, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if client2 != client1 {
		t.Error("Cache didn't return same client on second call")
	}
	if factoryCalls != 1 {
		t.Errorf("Factory calls = %d, want 1 (should not be called on cache hit)", factoryCalls)
	}
}

// TestIntegration_FieldManagerConsistency tests field manager naming consistency.
func TestIntegration_FieldManagerConsistency(t *testing.T) {
	controllerName := hivev1.ClustersyncControllerName

	// Get field manager name
	fieldManager := clientutil.FieldManagerName(controllerName)

	expectedFormat := "hive-clustersync"
	if fieldManager != expectedFormat {
		t.Errorf("FieldManagerName() = %q, want %q", fieldManager, expectedFormat)
	}

	// Verify consistency across multiple calls
	fieldManager2 := clientutil.FieldManagerName(controllerName)
	if fieldManager != fieldManager2 {
		t.Error("FieldManagerName() not consistent across calls")
	}
}

// TestIntegration_CacheInvalidationOnConfigChange tests automatic invalidation.
func TestIntegration_CacheInvalidationOnConfigChange(t *testing.T) {
	cache := clientutil.NewCache(
		clientutil.WithMaxSize(10),
		clientutil.WithTTL(1*time.Hour),
	)

	ctx := context.Background()
	factoryCalls := 0
	factory := func(ctx context.Context) (client.Client, error) {
		factoryCalls++
		return fake.NewClientBuilder().Build(), nil
	}

	clusterID := "hive/test-cluster"
	apiURL := "https://api-primary.example.com:6443"

	// Create client with version v1
	keyV1 := clientutil.NewCacheKey(clusterID, "v1", apiURL)
	client1, err := cache.Get(ctx, keyV1, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Simulate kubeconfig update (ResourceVersion changes)
	keyV2 := clientutil.NewCacheKey(clusterID, "v2", apiURL)
	client2, err := cache.Get(ctx, keyV2, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Should get different clients (automatic invalidation)
	if client1 == client2 {
		t.Error("Expected different clients after kubeconfig version change")
	}

	if factoryCalls != 2 {
		t.Errorf("Factory calls = %d, want 2 (version change should cause cache miss)", factoryCalls)
	}

	// Simulate API URL failover
	keySecondary := clientutil.NewCacheKey(clusterID, "v2", "https://api-secondary.example.com:6443")
	client3, err := cache.Get(ctx, keySecondary, factory)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Should get different client (automatic invalidation on URL change)
	if client2 == client3 {
		t.Error("Expected different clients after API URL change")
	}

	if factoryCalls != 3 {
		t.Errorf("Factory calls = %d, want 3 (URL change should cause cache miss)", factoryCalls)
	}
}

// TestIntegration_ConfigImmutability tests that config operations never mutate inputs.
func TestIntegration_ConfigImmutability(t *testing.T) {
	original := &rest.Config{
		Host:        "https://original.example.com:6443",
		BearerToken: "original-token",
	}

	// Apply multiple transformations
	withMetrics := clientutil.CopyConfigWithMetrics(original, hivev1.ClustersyncControllerName, false)
	withOverrides := clientutil.PrepareConfigForClient(withMetrics, "https://override.example.com:6443", "10.0.0.1")

	// Verify original is completely unchanged
	if original.Host != "https://original.example.com:6443" {
		t.Error("Original Host was mutated")
	}
	if original.BearerToken != "original-token" {
		t.Error("Original BearerToken was mutated")
	}
	if original.WrapTransport != nil {
		t.Error("Original WrapTransport was mutated")
	}
	if original.Dial != nil {
		t.Error("Original Dial was mutated")
	}

	// Verify transformations were applied to copies
	if withMetrics.Host != original.Host {
		t.Error("CopyConfigWithMetrics should preserve Host")
	}
	if withMetrics.WrapTransport == nil {
		t.Error("CopyConfigWithMetrics should apply wrapper")
	}

	if withOverrides.Host != "https://override.example.com:6443" {
		t.Error("PrepareConfigForClient should apply URL override")
	}
	if withOverrides.Dial == nil {
		t.Error("PrepareConfigForClient should apply IP override")
	}
}

// TestIntegration_EndToEnd tests a complete workflow using all components.
func TestIntegration_EndToEnd(t *testing.T) {
	// 1. Create infrastructure
	cache := clientutil.NewCache(
		clientutil.WithMaxSize(100),
		clientutil.WithTTL(10*time.Minute),
	)

	// 2. Prepare REST config
	cfg := &rest.Config{
		Host:        "https://test-cluster.example.com:6443",
		BearerToken: "test-token",
	}

	cfgWithMetrics := clientutil.CopyConfigWithMetrics(
		cfg,
		hivev1.ClustersyncControllerName,
		true, // remote cluster
	)

	cfgWithOverrides := clientutil.PrepareConfigForClient(
		cfgWithMetrics,
		"https://api-override.example.com:6443",
		"", // no IP override
	)

	// 3. Generate cache key
	cacheKey := clientutil.NewCacheKey(
		"hive/test-cluster",
		"v12345",
		cfgWithOverrides.Host,
	)

	// 4. Create client via cache
	ctx := context.Background()
	factory := func(ctx context.Context) (client.Client, error) {
		// In real usage, this would use cfgWithOverrides
		return fake.NewClientBuilder().Build(), nil
	}

	remoteClient, err := cache.Get(ctx, cacheKey, factory)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if remoteClient == nil {
		t.Fatal("Client is nil")
	}

	// 5. Simulate an operation error
	operationErr := context.DeadlineExceeded
	wrappedErr := clientutil.WrapClusterError(
		operationErr,
		cacheKey.ClusterID,
		"apply",
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
		"default",
		"test-deployment",
	)
	_ = wrappedErr // Error is wrapped but not tested further

	// 6. Get field manager name
	fieldManager := clientutil.FieldManagerName(hivev1.ClustersyncControllerName)
	if fieldManager != "hive-clustersync" {
		t.Errorf("Field manager = %q, want %q", fieldManager, "hive-clustersync")
	}

	t.Log("End-to-end integration test passed successfully")
}
